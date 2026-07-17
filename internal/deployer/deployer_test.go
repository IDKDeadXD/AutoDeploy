package deployer

import (
	"bytes"
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/idkde/deploy-agent/internal/model"
)

func TestCommandTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command test requires Unix shell")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := time.Now()
	err := run(ctx, t.TempDir(), model.Command{Command: "sleep 2"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("timed out command succeeded")
	}
	if time.Since(started) > 1500*time.Millisecond {
		t.Fatal("timeout was not enforced")
	}
}
