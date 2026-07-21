package cli

import (
	"strings"
	"testing"

	"github.com/aegmis/sandbox-cli/internal/runtime"
)

// TestDryRunInvariants is the cheap, no-Docker proof of the security invariant:
// the rendered docker command mounts only the project, sets the fake HOME, uses
// --rm, and never mounts the host home directory.
func TestDryRunInvariants(t *testing.T) {
	spec := runtime.RunSpec{
		Image:    "sandbox-base:0.1.0",
		Workdir:  "/workspace",
		Command:  []string{"echo", "hi there"},
		Remove:   true,
		Hostname: "sandbox",
		Home:     "/sandbox/home",
		User:     "root",
		Mounts:   []runtime.Mount{{Source: "/Users/dev/proj", Target: "/workspace"}},

		NoNewPrivileges: true,
		CapDrop:         []string{"ALL"},
		PidsLimit:       1024,
	}
	line := dockerCommandLine(spec)

	mustContain := []string{
		"--rm",
		"-e HOME=/sandbox/home",
		"-w /workspace",
		"type=bind,source=/Users/dev/proj,target=/workspace",
		"--security-opt no-new-privileges",
		"--cap-drop ALL",
		"--pids-limit 1024",
	}
	for _, s := range mustContain {
		if !strings.Contains(line, s) {
			t.Errorf("dry-run line missing %q:\n%s", s, line)
		}
	}

	// The only bind mount is the project; the host home is never mounted.
	if strings.Count(line, "type=bind") != 1 {
		t.Errorf("expected exactly one bind mount:\n%s", line)
	}
	// An argument with a space must be quoted so the line is copy-pasteable.
	if !strings.Contains(line, "'hi there'") {
		t.Errorf("expected quoted argument:\n%s", line)
	}
}
