package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/idkde/deploy-agent/internal/config"
	"github.com/idkde/deploy-agent/internal/daemon"
	"github.com/idkde/deploy-agent/internal/model"
	"github.com/idkde/deploy-agent/internal/notify"
	"github.com/idkde/deploy-agent/internal/secrets"
	"github.com/idkde/deploy-agent/internal/store"
	"github.com/idkde/deploy-agent/internal/updater"
)

var (
	Version = "0.1.0"
	Commit  = "unknown"
	Built   = "unknown"
)

type App struct {
	Store    store.Store
	In       io.Reader
	Out, Err io.Writer
}

func New() App { return App{Store: store.Default(), In: os.Stdin, Out: os.Stdout, Err: os.Stderr} }
func (a App) Run(args []string) error {
	if len(args) == 0 {
		return a.usage()
	}
	switch args[0] {
	case "daemon":
		return a.daemon()
	case "install":
		return a.install(args[1:])
	case "uninstall":
		return a.uninstall()
	case "update":
		return a.update(args[1:])
	case "init":
		return a.init()
	case "remove":
		return a.remove(args[1:])
	case "list":
		return a.list()
	case "status", "info":
		return a.status(args[1:])
	case "run":
		return a.run(args[1:])
	case "logs":
		return a.logs(args[1:])
	case "history":
		return a.history(args[1:])
	case "doctor":
		return a.doctor(args[1:])
	case "config":
		return a.config(args[1:])
	case "webhook":
		return a.webhook(args[1:])
	case "notifications":
		return a.notifications(args[1:])
	case "version":
		writeSection(a.Out, "Deploy Agent")
		writeFields(a.Out,
			[2]string{"Version", Version},
			[2]string{"Commit", Commit},
			[2]string{"Built", Built},
		)
		return nil
	case "help", "--help", "-h":
		return a.usage()
	default:
		return fmt.Errorf("unknown command %q\nrun 'deploy help' to see available commands", args[0])
	}
}
func (a App) daemon() error {
	c, err := a.Store.Config()
	if err != nil {
		return fmt.Errorf("read global config: %w", err)
	}
	ctx, stop := signalContext()
	defer stop()
	return daemon.New(a.Store, slog.New(slog.NewTextHandler(a.Err, nil)), c.MaxConcurrent).Serve(ctx, c)
}
func (a App) install(args []string) error {
	c := model.GlobalConfig{Listen: "0.0.0.0", Port: 4747, MaxConcurrent: 2}
	serviceUser := os.Getenv("SUDO_USER")
	for len(args) > 0 {
		switch args[0] {
		case "--port":
			if len(args) < 2 {
				return errors.New("--port requires a value")
			}
			v, e := strconv.Atoi(args[1])
			if e != nil || v < 1 || v > 65535 {
				return errors.New("invalid port")
			}
			c.Port = v
			args = args[2:]
		case "--listen":
			if len(args) < 2 {
				return errors.New("--listen requires a value")
			}
			c.Listen = args[1]
			args = args[2:]
		case "--public-url":
			if len(args) < 2 {
				return errors.New("--public-url requires a value")
			}
			c.PublicURL = strings.TrimRight(args[1], "/")
			args = args[2:]
		case "--user":
			if len(args) < 2 {
				return errors.New("--user requires a value")
			}
			serviceUser = args[1]
			args = args[2:]
		default:
			return fmt.Errorf("unknown install option %q", args[0])
		}
	}
	if os.Geteuid() != 0 {
		return errors.New("install must run as root")
	}
	if serviceUser == "" || serviceUser == "root" {
		return errors.New("install must name the non-root account that owns deployment repositories: sudo deploy install --user <name>")
	}
	account, err := user.Lookup(serviceUser)
	if err != nil {
		return fmt.Errorf("service user %q: %w", serviceUser, err)
	}
	c.ServiceUser = serviceUser
	c.BinaryPath = filepath.Join(account.HomeDir, ".local", "bin", "deploy")
	if err := installBinary(c.BinaryPath, account, "/proc/self/exe"); err != nil {
		return err
	}
	if err := a.Store.Ensure(); err != nil {
		return err
	}
	if err := a.Store.SaveConfig(c); err != nil {
		return err
	}
	unit := "[Unit]\nDescription=Deploy Agent\nAfter=network-online.target\n\n[Service]\nType=simple\nUser=" + account.Username + "\nGroup=" + account.Gid + "\nExecStart=" + c.BinaryPath + " daemon\nRestart=always\nNoNewPrivileges=true\nPrivateTmp=true\nProtectSystem=strict\nReadWritePaths=" + a.Store.Etc + " " + a.Store.Var + " " + a.Store.Log + " " + a.Store.Run + "\n\n[Install]\nWantedBy=multi-user.target\n"
	if err := os.WriteFile("/etc/systemd/system/deploy-agent.service", []byte(unit), 0644); err != nil {
		return err
	}
	for _, d := range []string{a.Store.Etc, a.Store.Var, a.Store.Log, a.Store.Run} {
		if err := exec.Command("chown", "-R", account.Uid+":"+account.Gid, d).Run(); err != nil {
			return fmt.Errorf("set ownership on %s: %w", d, err)
		}
	}
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return err
	}
	if err := exec.Command("systemctl", "enable", "--now", "deploy-agent").Run(); err != nil {
		return err
	}
	base := c.PublicURL
	if base == "" {
		base = "http://" + net.JoinHostPort(c.Listen, strconv.Itoa(c.Port))
	}
	writeSection(a.Out, "Deploy Agent installed")
	writeFields(a.Out,
		[2]string{"Service user", c.ServiceUser},
		[2]string{"Listen address", net.JoinHostPort(c.Listen, strconv.Itoa(c.Port))},
		[2]string{"Webhook base URL", base},
		[2]string{"Binary path", c.BinaryPath},
	)
	return nil
}
func installBinary(destination string, account *user.User, source string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return err
	}
	if err := os.Chown(filepath.Dir(destination), uid, gid); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".deploy-install-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := io.Copy(temporary, input); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0755); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chown(name, uid, gid); err != nil {
		return err
	}
	return os.Rename(name, destination)
}
func (a App) uninstall() error {
	if os.Geteuid() != 0 {
		return errors.New("uninstall must run as root")
	}
	_ = exec.Command("systemctl", "disable", "--now", "deploy-agent").Run()
	if err := os.Remove("/etc/systemd/system/deploy-agent.service"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return exec.Command("systemctl", "daemon-reload").Run()
}
func (a App) update(args []string) error {
	repository := ""
	if len(args) == 0 {
	} else if len(args) == 2 && args[0] == "--repo" {
		repository = args[1]
	} else {
		return errors.New("usage: deploy update [--repo owner/name]")
	}
	config, err := a.Store.Config()
	if err != nil {
		return fmt.Errorf("read global config: %w", err)
	}
	if config.BinaryPath == "" {
		return errors.New("this installation predates user-owned updates; rerun sudo deploy install --user " + config.ServiceUser)
	}
	result, err := updater.Update(repository, Version, config.BinaryPath)
	if err != nil {
		return err
	}
	if !result.Updated {
		fmt.Fprintf(a.Out, "[ok] Already up to date (%s).\n", result.Version)
		return nil
	}
	fmt.Fprintf(a.Out, "[ok] Updated to %s. Restarting daemon.\n", result.Version)
	return a.control(daemon.Control{Action: "restart"})
}
func (a App) init() error {
	root, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return errors.New("deploy init must run inside a Git repository")
	}
	root = stringTrim(root)
	remote, err := gitAt(root, "remote", "get-url", "origin")
	if err != nil {
		return fmt.Errorf("origin remote: %w", err)
	}
	branch, _ := gitAt(root, "branch", "--show-current")
	name := slug(filepath.Base(root))
	reader := bufio.NewReader(a.In)
	name = a.ask(reader, "Project name", name)
	branch = a.ask(reader, "Branch", stringTrim(branch))
	strategy := a.ask(reader, "Update strategy", "hard-reset")
	command := a.ask(reader, "Deployment command", "docker compose up -d --build --remove-orphans")
	if err := a.Store.Ensure(); err != nil {
		return err
	}
	if _, err := a.Store.Project(name); err == nil {
		return fmt.Errorf("project %q is already registered", name)
	}
	token, err := randomToken()
	if err != nil {
		return err
	}
	p := model.Project{Version: model.ConfigVersion, Name: name, Root: root, Token: token, Repository: model.Repository{Remote: "origin", Branch: branch, UpdateStrategy: strategy, URL: stringTrim(remote)}, Deployment: model.Deployment{WorkingDirectory: ".", StopOnFailure: true, Commands: []model.Command{{Name: "Deploy", Command: command, TimeoutSeconds: 900}}}}
	if err := config.ValidateProject(p); err != nil {
		return err
	}
	if err := a.Store.SaveProject(p); err != nil {
		return err
	}
	secret, err := secrets.New()
	if err != nil {
		return err
	}
	if err := a.Store.SaveSecret(name, "webhook", secret); err != nil {
		return err
	}
	if err := writeRepositoryConfig(p); err != nil {
		return err
	}
	c := model.GlobalConfig{Listen: "0.0.0.0", Port: 4747}
	if saved, e := a.Store.Config(); e == nil {
		c = saved
	}
	base := c.PublicURL
	if base == "" {
		base = "http://" + net.JoinHostPort(c.Listen, strconv.Itoa(c.Port))
	}
	fmt.Fprintln(a.Out)
	writeSection(a.Out, "Project registered")
	writeFields(a.Out,
		[2]string{"Project", p.Name},
		[2]string{"Repository", p.Repository.URL},
		[2]string{"Branch", p.Repository.Branch},
		[2]string{"Payload URL", fmt.Sprintf("%s/hooks/%s/%s", strings.TrimRight(base, "/"), p.Name, p.Token)},
		[2]string{"Content type", "application/json"},
		[2]string{"Secret", secret},
		[2]string{"Events", "Push events only"},
	)
	return nil
}
func (a App) ask(r *bufio.Reader, label, def string) string {
	fmt.Fprintf(a.Out, "%s [%s]: ", label, def)
	v, _ := r.ReadString('\n')
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	return v
}
func (a App) project(args []string) (model.Project, error) {
	name := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--project" && i+1 < len(args) {
			name = args[i+1]
		}
	}
	if name != "" {
		return a.Store.Project(name)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return model.Project{}, err
	}
	projects, err := a.Store.ListProjects()
	if err != nil {
		return model.Project{}, err
	}
	for _, p := range projects {
		if samePath(cwd, p.Root) {
			return p, nil
		}
	}
	return model.Project{}, errors.New("no registered project for this directory; use --project")
}
func (a App) remove(args []string) error {
	p, err := a.project(args)
	if err != nil {
		return err
	}
	if err := a.Store.RemoveProject(p.Name); err != nil {
		return err
	}
	for _, kind := range []string{"webhook", "discord"} {
		_ = os.Remove(filepath.Join(a.Store.Etc, "secrets", p.Name+"-"+kind+".secret"))
	}
	fmt.Fprintf(a.Out, "[ok] Removed project %s. Repository files were not changed.\n", p.Name)
	return nil
}
func (a App) list() error {
	projects, err := a.Store.ListProjects()
	if err != nil {
		return err
	}
	rows := [][]string{{"PROJECT", "BRANCH", "ROOT"}}
	for _, p := range projects {
		rows = append(rows, []string{p.Name, p.Repository.Branch, p.Root})
	}
	writeTable(a.Out, rows)
	return nil
}
func (a App) status(args []string) error {
	p, err := a.project(args)
	if err != nil {
		return err
	}
	st, err := a.Store.State(p.Name)
	if err != nil {
		return err
	}
	writeSection(a.Out, "Project status")
	writeFields(a.Out,
		[2]string{"Project", p.Name},
		[2]string{"Status", st.Status},
		[2]string{"Branch", p.Repository.Branch},
		[2]string{"Last detected", formatCommit(st.LastDetectedCommit)},
		[2]string{"Last attempted", formatCommit(st.LastAttemptedCommit)},
		[2]string{"Last successful", formatCommit(st.LastSuccessfulCommit)},
		[2]string{"Last failed", formatCommit(st.LastFailedCommit)},
	)
	if st.Pending != nil {
		fmt.Fprintln(a.Out)
		writeSection(a.Out, "Pending deployment")
		writeFields(a.Out,
			[2]string{"Commit", formatCommit(st.Pending.Commit)},
			[2]string{"Author", st.Pending.Author},
			[2]string{"Message", st.Pending.Message},
			[2]string{"Received", formatTime(st.Pending.ReceivedAt)},
		)
	}
	return nil
}
func (a App) run(args []string) error {
	p, err := a.project(args)
	if err != nil {
		return err
	}
	dry := has(args, "--dry-run")
	commit := value(args, "--commit")
	if dry {
		writeSection(a.Out, "Deployment preview")
		writeFields(a.Out,
			[2]string{"Project", p.Name},
			[2]string{"Remote", p.Repository.Remote},
			[2]string{"Branch", p.Repository.Branch},
			[2]string{"Git strategy", p.Repository.UpdateStrategy},
			[2]string{"Commit", emptyValue(commit)},
			[2]string{"Health check", formatBool(p.HealthCheck.Enabled)},
		)
		fmt.Fprintln(a.Out)
		fmt.Fprintln(a.Out, "Commands:")
		for _, c := range p.Deployment.Commands {
			target := c.Command
			if target == "" && c.Program != "" {
				target = strings.Join(append([]string{c.Program}, c.Args...), " ")
			}
			fmt.Fprintf(a.Out, "  - %s: %s\n", c.Name, emptyValue(target))
		}
		return nil
	}
	return a.control(daemon.Control{Action: "run", Project: p.Name, Commit: commit})
}
func (a App) history(args []string) error {
	p, err := a.project(args)
	if err != nil {
		return err
	}
	records, err := a.Store.Records(p.Name)
	if err != nil {
		return err
	}
	sortRecords(records)
	rows := [][]string{{"STATUS", "PROJECT", "COMMIT", "BRANCH", "DURATION", "STARTED"}}
	for _, r := range records {
		rows = append(rows, []string{r.Status, r.Project, formatCommit(r.Commit), r.Branch, formatDuration(r.DurationMillis), formatTime(r.StartedAt)})
	}
	writeTable(a.Out, rows)
	return nil
}
func (a App) logs(args []string) error {
	p, err := a.project(args)
	if err != nil {
		return err
	}
	records, err := a.Store.Records(p.Name)
	if err != nil {
		return err
	}
	rows := [][]string{{"STARTED", "ID", "STATUS", "ERROR"}}
	for _, r := range records {
		rows = append(rows, []string{formatTime(r.StartedAt), r.ID, r.Status, emptyValue(r.Error)})
	}
	writeTable(a.Out, rows)
	if has(args, "--follow") {
		return errors.New("log following requires journald: journalctl -fu deploy-agent")
	}
	return nil
}
func (a App) doctor(args []string) error {
	critical := false
	checks := []struct {
		name string
		ok   bool
	}{
		{"Global config", func() bool { _, e := a.Store.Config(); return e == nil }()},
		{"Git installed", func() bool { _, e := exec.LookPath("git"); return e == nil }()},
		{"Docker installed", func() bool { _, e := exec.LookPath("docker"); return e == nil }()},
		{"Deploy control socket", func() bool { _, e := os.Stat(filepath.Join(a.Store.Run, "deploy.sock")); return e == nil }()},
	}
	if p, e := a.project(args); e == nil {
		checks = append(checks, struct {
			name string
			ok   bool
		}{"Project configuration valid", config.ValidateProject(p) == nil})
	}
	for _, c := range checks {
		if !c.ok {
			critical = true
		}
		fmt.Fprintf(a.Out, "%s %s\n", formatCheck(c.ok), c.name)
	}
	if critical {
		return errors.New("one or more critical checks failed")
	}
	return nil
}
func (a App) config(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: deploy config <validate|command>")
	}
	switch args[0] {
	case "validate":
		p, err := a.project(args[1:])
		if err != nil {
			return err
		}
		if err := config.ValidateProject(p); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, "[ok] Configuration valid")
		return nil
	case "command":
		if len(args) < 2 || args[1] == "" {
			return errors.New("usage: deploy config command <shell-command> [--project name]")
		}
		p, err := a.project(args[2:])
		if err != nil {
			return err
		}
		p.Deployment.Commands[0].Command = args[1]
		p.Deployment.Commands[0].Program = ""
		p.Deployment.Commands[0].Args = nil
		if err := config.ValidateProject(p); err != nil {
			return err
		}
		if err := a.Store.SaveProject(p); err != nil {
			return err
		}
		if err := writeRepositoryConfig(p); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, "[ok] Deployment command updated")
		return nil
	default:
		return errors.New("usage: deploy config <validate|command>")
	}
}

func writeRepositoryConfig(p model.Project) error {
	repoConfig := filepath.Join(p.Root, "deploy", "config.json")
	if err := os.MkdirAll(filepath.Dir(repoConfig), 0755); err != nil {
		return err
	}
	public := struct {
		Version int `json:"version"`
		Project struct {
			Name string `json:"name"`
		} `json:"project"`
		Repository    model.Repository  `json:"repository"`
		Deployment    model.Deployment  `json:"deployment"`
		HealthCheck   model.HealthCheck `json:"healthCheck"`
		Notifications struct {
			Discord model.Discord `json:"discord"`
		} `json:"notifications"`
	}{Version: p.Version, Repository: p.Repository, Deployment: p.Deployment, HealthCheck: p.HealthCheck}
	public.Project.Name = p.Name
	public.Notifications.Discord = p.Discord
	data, err := json.MarshalIndent(public, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(repoConfig, append(data, '\n'), 0644)
}
func (a App) webhook(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: deploy webhook <show|reveal|rotate>")
	}
	p, err := a.project(args[1:])
	if err != nil {
		return err
	}
	secret, err := a.Store.Secret(p.Name, "webhook")
	if err != nil {
		return err
	}
	c, _ := a.Store.Config()
	base := c.PublicURL
	if base == "" {
		base = "http://" + net.JoinHostPort(c.Listen, strconv.Itoa(c.Port))
	}
	switch args[0] {
	case "show":
		writeSection(a.Out, "Webhook")
		writeFields(a.Out,
			[2]string{"URL", fmt.Sprintf("%s/hooks/%s/%s", base, p.Name, p.Token)},
			[2]string{"Secret", maskSecret(secret)},
		)
	case "reveal":
		if !has(args, "--yes") {
			return errors.New("refusing to reveal secret without --yes")
		}
		fmt.Fprintln(a.Out, secret)
	case "rotate":
		newSecret, e := secrets.New()
		if e != nil {
			return e
		}
		if e = a.Store.SaveSecret(p.Name, "webhook", newSecret); e != nil {
			return e
		}
		writeSection(a.Out, "New webhook secret")
		fmt.Fprintln(a.Out, newSecret)
	default:
		return errors.New("usage: deploy webhook <show|reveal|rotate>")
	}
	return nil
}
func (a App) notifications(args []string) error {
	if len(args) < 2 || args[0] != "discord" {
		return errors.New("usage: deploy notifications discord <setup|test|status|enable|disable|remove>")
	}
	p, err := a.project(args[2:])
	if err != nil {
		return err
	}
	switch args[1] {
	case "setup":
		r := bufio.NewReader(a.In)
		url := a.ask(r, "Discord webhook URL", "")
		if !strings.HasPrefix(url, "https://discord.com/api/webhooks/") {
			return errors.New("expected a Discord webhook URL")
		}
		p.Discord.Enabled = true
		p.Discord.Events = model.Events{Succeeded: true, Failed: true}
		if err := a.Store.SaveSecret(p.Name, "discord", url); err != nil {
			return err
		}
		if err := a.Store.SaveProject(p); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, "[ok] Discord notifications configured")
		return nil
	case "status":
		_, e := a.Store.Secret(p.Name, "discord")
		writeSection(a.Out, "Discord notifications")
		writeFields(a.Out,
			[2]string{"Status", formatBool(p.Discord.Enabled && e == nil)},
		)
		return nil
	case "enable":
		p.Discord.Enabled = true
		if err := a.Store.SaveProject(p); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, "[ok] Discord notifications enabled")
		return nil
	case "disable":
		p.Discord.Enabled = false
		if err := a.Store.SaveProject(p); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, "[ok] Discord notifications disabled")
		return nil
	case "remove":
		p.Discord.Enabled = false
		_ = os.Remove(filepath.Join(a.Store.Etc, "secrets", p.Name+"-discord.secret"))
		if err := a.Store.SaveProject(p); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, "[ok] Discord notifications removed")
		return nil
	case "test":
		url, e := a.Store.Secret(p.Name, "discord")
		if e != nil {
			return e
		}
		e = notify.Discord(context.Background(), url, model.Record{Project: p.Name, Status: "success", Commit: "manual", Branch: p.Repository.Branch, Message: "Deploy Agent notification test"})
		if e == nil {
			fmt.Fprintln(a.Out, "[ok] Discord test notification sent")
		}
		return e
	default:
		return errors.New("unknown Discord command")
	}
}
func (a App) control(c daemon.Control) error {
	conn, err := net.DialTimeout("unix", filepath.Join(a.Store.Run, "deploy.sock"), 2*time.Second)
	if err != nil {
		return fmt.Errorf("daemon unavailable: %w", err)
	}
	defer conn.Close()
	if err = json.NewEncoder(conn).Encode(c); err != nil {
		return err
	}
	var r daemon.Reply
	if err = json.NewDecoder(conn).Decode(&r); err != nil {
		return err
	}
	if r.Error != "" {
		return errors.New(r.Error)
	}
	if r.Message != "" {
		fmt.Fprintf(a.Out, "[ok] %s\n", r.Message)
	}
	return nil
}
func git(args ...string) (string, error) { return gitAt("", args...) }
func gitAt(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), stringTrim(string(out)))
	}
	return string(out), nil
}
func stringTrim(v string) string { return strings.TrimSpace(v) }
func slug(v string) string {
	v = strings.ToLower(v)
	var b strings.Builder
	for _, r := range v {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
func randomToken() (string, error) {
	s, err := secrets.New()
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(s, "dpl_"), nil
}
func has(args []string, v string) bool {
	for _, x := range args {
		if x == v {
			return true
		}
	}
	return false
}
func value(args []string, v string) string {
	for i, x := range args {
		if x == v && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
func short(v string) string {
	if len(v) > 7 {
		return v[:7]
	}
	return v
}
func samePath(a, b string) bool {
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	rel, err := filepath.Rel(bb, aa)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
func sortRecords(records []model.Record) {
	sort.Slice(records, func(i, j int) bool { return records[i].StartedAt.After(records[j].StartedAt) })
}
