// Package image lazily builds the embedded sandbox base image on first use.
package image

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/aegmis/sandbox-cli/internal/runtime"
)

//go:embed assets/Dockerfile
var dockerfile []byte

// Register wires the image builder into a DockerCLI runtime so EnsureImage can
// build a missing base image. Call this once at startup.
func Register(d *runtime.DockerCLI) {
	d.SetBuilder(func(ctx context.Context, ref string) error {
		return Build(ctx, d.Bin, ref)
	})
}

// Build builds ref from the embedded Dockerfile. The build context is a temp
// dir containing only the Dockerfile (the image needs no local files). Progress
// streams to stderr.
func Build(ctx context.Context, dockerBin, ref string) error {
	if dockerBin == "" {
		dockerBin = "docker"
	}

	tmp, err := os.MkdirTemp("", "sandbox-build-*")
	if err != nil {
		return fmt.Errorf("creating build context: %w", err)
	}
	defer os.RemoveAll(tmp)

	dfPath := filepath.Join(tmp, "Dockerfile")
	if err := os.WriteFile(dfPath, dockerfile, 0o644); err != nil {
		return fmt.Errorf("writing Dockerfile: %w", err)
	}

	fmt.Fprintf(os.Stderr, "sandbox-cli: building base image %s (first run only)...\n", ref)
	cmd := exec.CommandContext(ctx, dockerBin, "build", "-t", ref, "-f", dfPath, tmp)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	return nil
}
