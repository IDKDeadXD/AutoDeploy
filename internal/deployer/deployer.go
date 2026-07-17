package deployer

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/idkde/deploy-agent/internal/model"
)

type Result struct {
	FailedStep, Output, Error string
	ExitCode                  int
}
type Runner struct{ Redact func(string) string }

func (r Runner) Run(ctx context.Context, p model.Project, commit string, dry bool) (Result, error) {
	if dry {
		return Result{}, nil
	}
	work := filepath.Join(p.Root, p.Deployment.WorkingDirectory)
	if p.Deployment.WorkingDirectory == "" {
		work = p.Root
	}
	if err := git(ctx, p.Root, "fetch", p.Repository.Remote, p.Repository.Branch); err != nil {
		return Result{FailedStep: "Fetch repository", Error: err.Error()}, err
	}
	remote, err := gitOutput(ctx, p.Root, "rev-parse", p.Repository.Remote+"/"+p.Repository.Branch)
	if err != nil {
		return Result{FailedStep: "Resolve remote commit", Error: err.Error()}, err
	}
	if commit != "" && remote != commit {
		return Result{FailedStep: "Verify remote commit", Error: "fetched commit differs from requested commit"}, fmt.Errorf("fetched commit %s differs from requested commit %s", remote, commit)
	}
	if p.Repository.UpdateStrategy == "hard-reset" {
		err = git(ctx, p.Root, "reset", "--hard", p.Repository.Remote+"/"+p.Repository.Branch)
	} else {
		err = git(ctx, p.Root, "merge", "--ff-only", p.Repository.Remote+"/"+p.Repository.Branch)
	}
	if err != nil {
		return Result{FailedStep: "Update repository", Error: err.Error()}, err
	}
	var output bytes.Buffer
	for _, c := range p.Deployment.Commands {
		commandCtx, cancel := context.WithTimeout(ctx, time.Duration(c.TimeoutSeconds)*time.Second)
		err := run(commandCtx, work, c, &output)
		timedOut := commandCtx.Err() == context.DeadlineExceeded
		cancel()
		if err != nil {
			result := Result{FailedStep: c.Name, Output: r.clean(output.String()), Error: err.Error()}
			if timedOut {
				result.Error = "command timed out"
			}
			if e, ok := err.(*exec.ExitError); ok {
				result.ExitCode = e.ExitCode()
			}
			return result, err
		}
	}
	if p.HealthCheck.Enabled {
		if err := health(ctx, p.HealthCheck); err != nil {
			return Result{FailedStep: "Health check", Output: r.clean(output.String()), Error: err.Error()}, err
		}
	}
	return Result{Output: r.clean(output.String())}, nil
}
func (r Runner) clean(v string) string {
	if r.Redact != nil {
		return r.Redact(v)
	}
	return v
}
func git(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %s", args, bytes.TrimSpace(out))
	}
	return nil
}
func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %v: %s", args, bytes.TrimSpace(out))
	}
	return string(bytes.TrimSpace(out)), nil
}
func run(ctx context.Context, dir string, c model.Command, w io.Writer) error {
	var cmd *exec.Cmd
	if c.Program != "" {
		cmd = exec.CommandContext(ctx, c.Program, c.Args...)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", c.Command)
	}
	cmd.Dir = dir
	cmd.Stdout = w
	cmd.Stderr = w
	prepareProcess(cmd)
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		killProcessGroup(cmd)
	}
	return err
}
func health(ctx context.Context, h model.HealthCheck) error {
	deadline := time.Now().Add(time.Duration(h.TimeoutSeconds) * time.Second)
	interval := time.Duration(h.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 3 * time.Second
	}
	client := http.Client{Timeout: 10 * time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.URL, nil)
		if err == nil {
			res, e := client.Do(req)
			if e == nil {
				_, _ = io.Copy(io.Discard, res.Body)
				res.Body.Close()
				if res.StatusCode == h.ExpectedStatus {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("health check did not return %d before timeout", h.ExpectedStatus)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
func NewID() string { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
