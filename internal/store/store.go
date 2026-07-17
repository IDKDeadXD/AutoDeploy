package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/idkde/deploy-agent/internal/model"
)

type Store struct{ Etc, Var, Log, Run string }

func Default() Store {
	return Store{etc("DEPLOY_AGENT_ETC", "/etc/deploy-agent"), etc("DEPLOY_AGENT_VAR", "/var/lib/deploy-agent"), etc("DEPLOY_AGENT_LOG", "/var/log/deploy-agent"), etc("DEPLOY_AGENT_RUN", "/run/deploy-agent")}
}
func etc(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func (s Store) Ensure() error {
	for _, p := range []string{s.Etc, s.Var, s.Log, s.Run, s.projects(), s.secrets(), s.state(), s.history()} {
		if err := os.MkdirAll(p, 0700); err != nil {
			return err
		}
	}
	return nil
}
func (s Store) projects() string { return filepath.Join(s.Etc, "projects") }
func (s Store) secrets() string  { return filepath.Join(s.Etc, "secrets") }
func (s Store) state() string    { return filepath.Join(s.Var, "state") }
func (s Store) history() string  { return filepath.Join(s.Var, "history") }
func safeName(v string) (string, error) {
	if v == "" || len(v) > 64 {
		return "", errors.New("project name must be 1-64 characters")
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return "", errors.New("project name may contain only lowercase letters, digits, hyphens, and underscores")
		}
	}
	return v, nil
}
func writeJSON(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomic(path, append(data, '\n'), mode)
}
func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
func atomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".new-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
func (s Store) Config() (model.GlobalConfig, error) {
	var c model.GlobalConfig
	err := readJSON(filepath.Join(s.Etc, "config.json"), &c)
	return c, err
}
func (s Store) SaveConfig(c model.GlobalConfig) error {
	return writeJSON(filepath.Join(s.Etc, "config.json"), c, 0600)
}
func (s Store) Project(name string) (model.Project, error) {
	var p model.Project
	n, err := safeName(name)
	if err != nil {
		return p, err
	}
	err = readJSON(filepath.Join(s.projects(), n+".json"), &p)
	return p, err
}
func (s Store) SaveProject(p model.Project) error {
	n, err := safeName(p.Name)
	if err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.projects(), n+".json"), p, 0600)
}
func (s Store) ListProjects() ([]model.Project, error) {
	entries, err := os.ReadDir(s.projects())
	if err != nil {
		return nil, err
	}
	out := []model.Project{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p, err := s.Project(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}
func (s Store) RemoveProject(name string) error {
	n, err := safeName(name)
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(s.projects(), n+".json"))
}
func (s Store) Secret(name, kind string) (string, error) {
	n, err := safeName(name)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(s.secrets(), n+"-"+kind+".secret"))
	return strings.TrimSpace(string(b)), err
}
func (s Store) SaveSecret(name, kind, value string) error {
	n, err := safeName(name)
	if err != nil {
		return err
	}
	if strings.ContainsAny(value, "\r\n") {
		return errors.New("secret contains newline")
	}
	return atomic(filepath.Join(s.secrets(), n+"-"+kind+".secret"), []byte(value+"\n"), 0600)
}
func (s Store) State(name string) (model.State, error) {
	var st model.State
	err := readJSON(filepath.Join(s.state(), name+".json"), &st)
	if os.IsNotExist(err) {
		return model.State{Status: "idle"}, nil
	}
	return st, err
}
func (s Store) SaveState(name string, st model.State) error {
	return writeJSON(filepath.Join(s.state(), name+".json"), st, 0600)
}
func (s Store) AddRecord(r model.Record) error {
	return writeJSON(filepath.Join(s.history(), r.ID+".json"), r, 0600)
}
func (s Store) Records(project string) ([]model.Record, error) {
	entries, err := os.ReadDir(s.history())
	if err != nil {
		return nil, err
	}
	out := []model.Record{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var r model.Record
		if err := readJSON(filepath.Join(s.history(), e.Name()), &r); err != nil {
			return nil, fmt.Errorf("read history: %w", err)
		}
		if project == "" || r.Project == project {
			out = append(out, r)
		}
	}
	return out, nil
}
