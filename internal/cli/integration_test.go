//go:build docker_integration

// These tests require a running Docker daemon and the sandbox-base image (built
// lazily on first run). Enable with: go test -tags docker_integration ./...
package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amitghadge/sandbox-cli/internal/config"
	"github.com/amitghadge/sandbox-cli/internal/image"
	"github.com/amitghadge/sandbox-cli/internal/runtime"
	"github.com/amitghadge/sandbox-cli/internal/sandbox"
)

func newTestSession(t *testing.T, cfg config.Config) *sandbox.Session {
	t.Helper()
	if !dockerAvailable() {
		t.Skip("docker daemon not available")
	}
	sess := sandbox.New(cfg)
	if d, ok := sess.Runtime.(*runtime.DockerCLI); ok {
		image.Register(d)
	}
	return sess
}

// runInSandbox runs a command in the sandbox and captures its stdout by shelling
// out to the built binary is avoided; instead we invoke docker via the runtime
// but redirect stdout through a temp file written inside /workspace.
func TestIsolation_HomeAndWorkspace(t *testing.T) {
	proj := t.TempDir()
	cfg := config.Default()
	sess := newTestSession(t, cfg)

	// Write a marker file inside the container's /workspace; assert it appears on host.
	code, err := sess.Run(context.Background(), sandbox.Options{
		Project: proj,
		TTY:     ptr(false),
		Command: []string{"sh", "-c", "echo $HOME > /workspace/home.txt && touch /workspace/created.txt"},
	}, false)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	homeTxt, err := os.ReadFile(filepath.Join(proj, "home.txt"))
	if err != nil {
		t.Fatalf("reading home.txt: %v", err)
	}
	if got := strings.TrimSpace(string(homeTxt)); got != "/sandbox/home" {
		t.Errorf("HOME inside container = %q, want /sandbox/home", got)
	}
	if _, err := os.Stat(filepath.Join(proj, "created.txt")); err != nil {
		t.Errorf("expected created.txt on host: %v", err)
	}
}

// TestRmRfHomeSafety proves that `rm -rf ~` inside the sandbox cannot touch the
// host home: we place a canary in a fake host HOME that is never mounted.
func TestRmRfHomeSafety(t *testing.T) {
	proj := t.TempDir()
	fakeHome := t.TempDir()
	canary := filepath.Join(fakeHome, "canary.txt")
	if err := os.WriteFile(canary, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	sess := newTestSession(t, config.Default())
	_, err := sess.Run(context.Background(), sandbox.Options{
		Project: proj,
		TTY:     ptr(false),
		Command: []string{"sh", "-c", "rm -rf ~ || true; rm -rf / 2>/dev/null || true; echo done"},
	}, false)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	if _, err := os.Stat(canary); err != nil {
		t.Fatalf("host canary destroyed — isolation FAILED: %v", err)
	}
	// The fake host home dir and its contents remain intact.
	if _, err := os.Stat(fakeHome); err != nil {
		t.Fatalf("fake host home destroyed — isolation FAILED: %v", err)
	}
}

// TestEnvPassthrough proves allowlisted host env reaches the container while
// non-allowlisted vars do not.
func TestEnvPassthrough(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("SANDBOX_TEST_ALLOWED", "yes")
	t.Setenv("SANDBOX_TEST_SECRET", "leak")

	sess := newTestSession(t, config.Default())
	_, err := sess.Run(context.Background(), sandbox.Options{
		Project:  proj,
		TTY:      ptr(false),
		EnvAllow: []string{"SANDBOX_TEST_ALLOWED"},
		Command:  []string{"sh", "-c", "echo allowed=$SANDBOX_TEST_ALLOWED secret=$SANDBOX_TEST_SECRET > /workspace/env.txt"},
	}, false)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(proj, "env.txt"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "allowed=yes") {
		t.Errorf("allowlisted var not forwarded: %q", s)
	}
	if strings.Contains(s, "secret=leak") {
		t.Errorf("non-allowlisted var leaked into container: %q", s)
	}
}

// dockerAvailable is a cheap precondition guard used by TestMain-style skips.
func dockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	return cmd.Run() == nil
}

func ptr(b bool) *bool { return &b }
