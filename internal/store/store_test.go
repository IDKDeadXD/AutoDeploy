package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSecretPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not report Unix mode bits")
	}
	root := t.TempDir()
	s := Store{Etc: filepath.Join(root, "etc"), Var: filepath.Join(root, "var"), Log: filepath.Join(root, "log"), Run: filepath.Join(root, "run")}
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSecret("app", "webhook", "value"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(s.Etc, "secrets", "app-webhook.secret"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatalf("secret is too permissive: %o", info.Mode().Perm())
	}
}
