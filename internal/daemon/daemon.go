package daemon

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/idkde/deploy-agent/internal/deployer"
	"github.com/idkde/deploy-agent/internal/model"
	"github.com/idkde/deploy-agent/internal/notify"
	"github.com/idkde/deploy-agent/internal/store"
	"github.com/idkde/deploy-agent/internal/webhook"
)

type Daemon struct {
	Store      store.Store
	Log        *slog.Logger
	mu         sync.Mutex
	workers    map[string]*worker
	deliveries map[string]time.Time
	sem        chan struct{}
}
type worker struct {
	running bool
	pending *model.Job
}
type Control struct {
	Action  string `json:"action"`
	Project string `json:"project"`
	Commit  string `json:"commit"`
	DryRun  bool   `json:"dryRun"`
}
type Reply struct {
	Error    string          `json:"error,omitempty"`
	Projects []model.Project `json:"projects,omitempty"`
	State    *model.State    `json:"state,omitempty"`
	Records  []model.Record  `json:"records,omitempty"`
	Message  string          `json:"message,omitempty"`
}

func New(s store.Store, log *slog.Logger, max int) *Daemon {
	if max < 1 {
		max = 2
	}
	return &Daemon{Store: s, Log: log, workers: map[string]*worker{}, deliveries: map[string]time.Time{}, sem: make(chan struct{}, max)}
}
func (d *Daemon) Serve(ctx context.Context, c model.GlobalConfig) error {
	if err := d.Store.Ensure(); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", d.health)
	mux.HandleFunc("POST /hooks/", d.hook)
	server := &http.Server{Addr: net.JoinHostPort(c.Listen, fmt.Sprint(c.Port)), Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 8192}
	go d.control(ctx)
	go d.recover(ctx)
	errc := make(chan error, 1)
	go func() { errc <- server.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	case err := <-errc:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}
func (d *Daemon) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, "{\"status\":\"ok\"}\n")
}
func (d *Daemon) hook(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/hooks/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.NotFound(w, r)
		return
	}
	p, err := d.Store.Project(parts[0])
	if err != nil || subtle.ConstantTimeCompare([]byte(p.Token), []byte(parts[1])) != 1 {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	secret, err := d.Store.Secret(p.Name, "webhook")
	if err != nil || !webhook.Verify(secret, body, r.Header.Get("X-Hub-Signature-256")) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	if r.Header.Get("X-GitHub-Event") != "push" {
		http.Error(w, "unsupported event", http.StatusUnprocessableEntity)
		return
	}
	job, err := webhook.ParsePush(body, p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusAccepted)
		return
	}
	job.DeliveryID = r.Header.Get("X-GitHub-Delivery")
	job.ReceivedAt = time.Now().UTC()
	if job.DeliveryID == "" {
		http.Error(w, "missing delivery id", http.StatusBadRequest)
		return
	}
	if d.duplicate(job.DeliveryID) {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	d.enqueue(p, job)
	w.WriteHeader(http.StatusAccepted)
}
func (d *Daemon) duplicate(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	for k, v := range d.deliveries {
		if now.Sub(v) > 24*time.Hour {
			delete(d.deliveries, k)
		}
	}
	if _, ok := d.deliveries[id]; ok {
		return true
	}
	d.deliveries[id] = now
	return false
}
func (d *Daemon) enqueue(p model.Project, j model.Job) {
	d.mu.Lock()
	w := d.workers[p.Name]
	if w == nil {
		w = &worker{}
		d.workers[p.Name] = w
	}
	st, _ := d.Store.State(p.Name)
	st.LastDetectedCommit = j.Commit
	if w.running {
		w.pending = &j
		st.Pending = &j
		_ = d.Store.SaveState(p.Name, st)
		d.mu.Unlock()
		return
	}
	w.running = true
	st.Pending = nil
	_ = d.Store.SaveState(p.Name, st)
	d.mu.Unlock()
	go d.execute(p, j)
}
func (d *Daemon) execute(p model.Project, j model.Job) {
	d.sem <- struct{}{}
	started := time.Now().UTC()
	st, _ := d.Store.State(p.Name)
	st.Status = "deploying"
	st.LastAttemptedCommit = j.Commit
	st.Pending = nil
	_ = d.Store.SaveState(p.Name, st)
	if p.Discord.Enabled && p.Discord.Events.Started {
		d.discord(context.Background(), p, model.Record{Project: p.Name, Status: "started", Commit: j.Commit, Branch: p.Repository.Branch, Message: j.Message})
	}
	result, err := (deployer.Runner{Redact: func(v string) string { return redact(v) }}).Run(context.Background(), p, j.Commit, false)
	ended := time.Now().UTC()
	record := model.Record{ID: deployer.NewID(), Project: p.Name, Commit: j.Commit, Branch: p.Repository.Branch, Author: j.Author, Message: j.Message, FailedStep: result.FailedStep, Error: result.Error, ExitCode: result.ExitCode, StartedAt: started, EndedAt: ended, DurationMillis: ended.Sub(started).Milliseconds()}
	if err == nil {
		record.Status = "success"
		st.Status = "idle"
		st.LastSuccessfulCommit = j.Commit
	} else {
		record.Status = "failed"
		st.Status = "failed"
		st.LastFailedCommit = j.Commit
	}
	_ = d.Store.AddRecord(record)
	_ = d.Store.SaveState(p.Name, st)
	if p.Discord.Enabled && ((err == nil && p.Discord.Events.Succeeded) || (err != nil && p.Discord.Events.Failed)) {
		d.discord(context.Background(), p, record)
	}
	<-d.sem
	d.mu.Lock()
	w := d.workers[p.Name]
	next := w.pending
	w.pending = nil
	if next == nil {
		w.running = false
	}
	d.mu.Unlock()
	if next != nil {
		d.execute(p, *next)
	}
}
func redact(v string) string {
	return strings.ReplaceAll(v, "Authorization:", "Authorization: [REDACTED]")
}
func (d *Daemon) discord(ctx context.Context, p model.Project, r model.Record) {
	url, err := d.Store.Secret(p.Name, "discord")
	if err != nil {
		return
	}
	if err := notify.Discord(ctx, url, r); err != nil {
		d.Log.Warn("Discord notification failed", "project", p.Name, "error", err)
	}
}
func (d *Daemon) recover(ctx context.Context) {
	projects, err := d.Store.ListProjects()
	if err != nil {
		return
	}
	for _, p := range projects {
		st, err := d.Store.State(p.Name)
		if err == nil && st.Pending != nil {
			d.enqueue(p, *st.Pending)
		}
	}
}
func (d *Daemon) control(ctx context.Context) {
	path := filepath.Join(d.Store.Run, "deploy.sock")
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		d.Log.Error("control socket unavailable", "error", err)
		return
	}
	_ = os.Chmod(path, 0660)
	defer os.Remove(path)
	go func() { <-ctx.Done(); _ = ln.Close() }()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go d.serveControl(conn)
	}
}
func (d *Daemon) serveControl(c net.Conn) {
	defer c.Close()
	var request Control
	if err := json.NewDecoder(io.LimitReader(c, 8192)).Decode(&request); err != nil {
		_ = json.NewEncoder(c).Encode(Reply{Error: "invalid request"})
		return
	}
	reply := d.handle(request)
	_ = json.NewEncoder(c).Encode(reply)
}
func (d *Daemon) handle(c Control) Reply {
	switch c.Action {
	case "list":
		p, e := d.Store.ListProjects()
		return Reply{Projects: p, Error: errorText(e)}
	case "status":
		s, e := d.Store.State(c.Project)
		return Reply{State: &s, Error: errorText(e)}
	case "history":
		r, e := d.Store.Records(c.Project)
		sort.Slice(r, func(i, j int) bool { return r[i].StartedAt.After(r[j].StartedAt) })
		return Reply{Records: r, Error: errorText(e)}
	case "run":
		p, e := d.Store.Project(c.Project)
		if e != nil {
			return Reply{Error: errorText(e)}
		}
		if c.DryRun {
			return Reply{Message: "dry run: " + p.Repository.Remote + " " + p.Repository.Branch}
		}
		d.enqueue(p, model.Job{Commit: c.Commit, ReceivedAt: time.Now().UTC()})
		return Reply{Message: "deployment queued"}
	default:
		return Reply{Error: "unknown action"}
	}
}
func errorText(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}
