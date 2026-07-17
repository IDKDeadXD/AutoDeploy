package config

import (
	"testing"

	"github.com/idkde/deploy-agent/internal/model"
)

func TestValidateProject(t *testing.T) {
	root := t.TempDir()
	p := model.Project{Version: 1, Name: "app", Root: root, Token: "0123456789012345", Repository: model.Repository{Remote: "origin", Branch: "main", UpdateStrategy: "hard-reset"}, Deployment: model.Deployment{Commands: []model.Command{{Name: "start", Program: "true", TimeoutSeconds: 1}}}}
	if err := ValidateProject(p); err != nil {
		t.Fatal(err)
	}
	p.Repository.UpdateStrategy = "unsafe"
	if err := ValidateProject(p); err == nil {
		t.Fatal("invalid strategy accepted")
	}
}
func TestRemoteMatches(t *testing.T) {
	if !RemoteMatches("git@github.com:acme/app.git", "https://github.com/acme/app.git") {
		t.Fatal("equivalent remotes did not match")
	}
}
