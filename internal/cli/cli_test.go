package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/idkde/deploy-agent/internal/model"
	"github.com/idkde/deploy-agent/internal/store"
)

func TestUsageShowsGroupedHelp(t *testing.T) {
	var out bytes.Buffer
	app := App{Out: &out}

	if err := app.Run([]string{"help"}); err != nil {
		t.Fatal(err)
	}

	text := out.String()
	for _, want := range []string{
		"Deploy Agent",
		"Usage:",
		"Setup:",
		"Deployments:",
		"notifications discord setup",
		"Examples:",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("help output missing %q:\n%s", want, text)
		}
	}
}

func TestListWritesAlignedProjectTable(t *testing.T) {
	app := testApp(t)
	saveProject(t, app.Store, model.Project{Name: "flux", Root: "/srv/flux", Repository: model.Repository{Branch: "main"}})
	saveProject(t, app.Store, model.Project{Name: "site", Root: "/srv/site", Repository: model.Repository{Branch: "prod"}})

	if err := app.Run([]string{"list"}); err != nil {
		t.Fatal(err)
	}

	text := app.Out.(*bytes.Buffer).String()
	for _, want := range []string{"PROJECT", "BRANCH", "ROOT", "flux", "main", "/srv/site"} {
		if !strings.Contains(text, want) {
			t.Fatalf("list output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\t") {
		t.Fatalf("list output should be expanded for display, got tabs:\n%s", text)
	}
}

func TestStatusWritesSectionedState(t *testing.T) {
	app := testApp(t)
	saveProject(t, app.Store, model.Project{Name: "flux", Root: "/srv/flux", Repository: model.Repository{Branch: "main"}})
	if err := app.Store.SaveState("flux", model.State{
		Status:               "failed",
		LastSuccessfulCommit: "1234567890abcdef",
		Pending: &model.Job{
			Commit:     "abcdef1234567890",
			Author:     "deploybot",
			Message:    "ship it",
			ReceivedAt: time.Date(2026, 7, 17, 12, 30, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := app.Run([]string{"status", "--project", "flux"}); err != nil {
		t.Fatal(err)
	}

	text := app.Out.(*bytes.Buffer).String()
	for _, want := range []string{"Project status", "Project:", "flux", "Last successful:", "1234567", "Pending deployment", "abcdef1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("status output missing %q:\n%s", want, text)
		}
	}
}

func TestRunDryRunShowsDeploymentPreview(t *testing.T) {
	app := testApp(t)
	saveProject(t, app.Store, model.Project{
		Name:       "flux",
		Root:       "/srv/flux",
		Repository: model.Repository{Remote: "origin", Branch: "main", UpdateStrategy: "hard-reset"},
		Deployment: model.Deployment{Commands: []model.Command{
			{Name: "Build", Program: "docker", Args: []string{"compose", "up", "-d"}},
		}},
		HealthCheck: model.HealthCheck{Enabled: true},
	})

	if err := app.Run([]string{"run", "--project", "flux", "--commit", "abcdef1", "--dry-run"}); err != nil {
		t.Fatal(err)
	}

	text := app.Out.(*bytes.Buffer).String()
	for _, want := range []string{"Deployment preview", "Git strategy:", "hard-reset", "Health check:", "enabled", "Build: docker compose up -d"} {
		if !strings.Contains(text, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, text)
		}
	}
}

func TestWebhookShowMasksSecret(t *testing.T) {
	app := testApp(t)
	saveProject(t, app.Store, model.Project{Name: "flux", Root: "/srv/flux", Token: "hook-token"})
	if err := app.Store.SaveSecret("flux", "webhook", "1234567890abcdef"); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveConfig(model.GlobalConfig{PublicURL: "https://deploy.example.com"}); err != nil {
		t.Fatal(err)
	}

	if err := app.Run([]string{"webhook", "show", "--project", "flux"}); err != nil {
		t.Fatal(err)
	}

	text := app.Out.(*bytes.Buffer).String()
	for _, want := range []string{"Webhook", "https://deploy.example.com/hooks/flux/hook-token", "12345678...cdef"} {
		if !strings.Contains(text, want) {
			t.Fatalf("webhook output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "90ab") {
		t.Fatalf("webhook output leaked the middle of the secret:\n%s", text)
	}
}

func testApp(t *testing.T) App {
	t.Helper()
	root := t.TempDir()
	s := store.Store{
		Etc: filepath.Join(root, "etc"),
		Var: filepath.Join(root, "var"),
		Log: filepath.Join(root, "log"),
		Run: filepath.Join(root, "run"),
	}
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	return App{Store: s, Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
}

func saveProject(t *testing.T, s store.Store, p model.Project) {
	t.Helper()
	if err := s.SaveProject(p); err != nil {
		t.Fatal(err)
	}
}
