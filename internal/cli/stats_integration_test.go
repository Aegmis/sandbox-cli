//go:build docker_integration

package cli

import (
	"os/exec"
	"testing"
	"time"
)

// TestCollectSandboxStats starts a sandbox-named container and asserts the stats
// collector reports it with a memory reading.
func TestCollectSandboxStats(t *testing.T) {
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("docker daemon not available")
	}

	name := "sandbox-statstest-" + time.Now().Format("150405.000")
	if err := exec.Command("docker", "run", "-d", "--rm", "--name", name, "alpine", "sleep", "30").Run(); err != nil {
		t.Fatalf("starting test container: %v", err)
	}
	defer exec.Command("docker", "rm", "-f", name).Run()

	// Give docker a moment to register the container.
	time.Sleep(1 * time.Second)

	rows, err := collectSandboxStats("docker")
	if err != nil {
		t.Fatalf("collectSandboxStats: %v", err)
	}
	var found *statRow
	for i := range rows {
		if rows[i].Name == name {
			found = &rows[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("container %q not found in stats rows: %+v", name, rows)
	}
	if found.Mem == "" || found.CPU == "" {
		t.Errorf("empty stats for %q: %+v", name, *found)
	}
}
