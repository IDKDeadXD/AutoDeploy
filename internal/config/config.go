package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/idkde/deploy-agent/internal/model"
)

func ValidateProject(p model.Project) error {
	if p.Version != model.ConfigVersion {
		return fmt.Errorf("unsupported project config version %d", p.Version)
	}
	if p.Name == "" || p.Root == "" || !filepath.IsAbs(p.Root) {
		return errors.New("project name and absolute repository root are required")
	}
	info, err := os.Stat(p.Root)
	if err != nil {
		return fmt.Errorf("repository root: %w", err)
	}
	if !info.IsDir() {
		return errors.New("repository root is not a directory")
	}
	if p.Token == "" || len(p.Token) < 16 {
		return errors.New("webhook token is invalid")
	}
	if p.Repository.Remote == "" || p.Repository.Branch == "" {
		return errors.New("repository remote and branch are required")
	}
	if p.Repository.UpdateStrategy != "hard-reset" && p.Repository.UpdateStrategy != "fast-forward" {
		return errors.New("update strategy must be hard-reset or fast-forward")
	}
	if len(p.Deployment.Commands) == 0 {
		return errors.New("at least one deployment command is required")
	}
	for i, c := range p.Deployment.Commands {
		if c.Name == "" {
			return fmt.Errorf("command %d has no name", i+1)
		}
		if c.TimeoutSeconds <= 0 {
			return fmt.Errorf("command %q timeout must be positive", c.Name)
		}
		if c.Program == "" && c.Command == "" {
			return fmt.Errorf("command %q has no program or shell command", c.Name)
		}
		if c.Program != "" && c.Command != "" {
			return fmt.Errorf("command %q cannot use both program and shell command", c.Name)
		}
	}
	if p.HealthCheck.Enabled {
		u, err := url.ParseRequestURI(p.HealthCheck.URL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return errors.New("health check URL must be absolute")
		}
		if p.HealthCheck.TimeoutSeconds <= 0 {
			return errors.New("health check timeout must be positive")
		}
		if p.HealthCheck.ExpectedStatus == 0 {
			p.HealthCheck.ExpectedStatus = 200
		}
	}
	return nil
}
func RemoteMatches(configured, payload string) bool {
	return normalizeRemote(configured) == normalizeRemote(payload)
}
func normalizeRemote(v string) string {
	v = strings.TrimSpace(strings.TrimSuffix(v, ".git"))
	v = strings.TrimPrefix(v, "https://")
	v = strings.TrimPrefix(v, "http://")
	v = strings.TrimPrefix(v, "ssh://")
	v = strings.TrimPrefix(v, "git@")
	return strings.ToLower(strings.TrimSuffix(strings.Replace(v, ":", "/", 1), "/"))
}
