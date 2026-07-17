package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// DockerCLI is the MVP Runtime backend. It shells out to the `docker` binary,
// inheriting the real stdio file descriptors so `docker run -it` handles pty /
// raw-mode natively — no pty library or manual attach/resize plumbing needed.
type DockerCLI struct {
	// Bin is the docker executable name/path. Defaults to "docker".
	Bin string
	// Stderr receives image-build progress and diagnostics. Defaults to os.Stderr.
	Stderr *os.File
	// builder builds a missing image; wired by the image package via SetBuilder
	// to avoid an import cycle.
	builder Builder
}

// NewDockerCLI returns a DockerCLI with sensible defaults.
func NewDockerCLI() *DockerCLI {
	return &DockerCLI{Bin: "docker", Stderr: os.Stderr}
}

func (d *DockerCLI) bin() string {
	if d.Bin == "" {
		return "docker"
	}
	return d.Bin
}

func (d *DockerCLI) stderr() *os.File {
	if d.Stderr == nil {
		return os.Stderr
	}
	return d.Stderr
}

// Available checks that docker is on PATH and the daemon is reachable.
func (d *DockerCLI) Available(ctx context.Context) error {
	if _, err := exec.LookPath(d.bin()); err != nil {
		return fmt.Errorf("docker not found on PATH: %w", err)
	}
	cmd := exec.CommandContext(ctx, d.bin(), "info", "--format", "{{.ServerVersion}}")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker daemon not reachable (is Docker Desktop running?): %s", string(out))
	}
	return nil
}

// EnsureImage checks whether ref exists locally. The actual build is delegated
// to a Builder set by the image package to avoid an import cycle; if none is
// set, a missing image is reported as an error.
func (d *DockerCLI) EnsureImage(ctx context.Context, ref string, forceBuild bool) error {
	if !forceBuild {
		insp := exec.CommandContext(ctx, d.bin(), "image", "inspect", ref)
		if err := insp.Run(); err == nil {
			return nil // already present
		}
	}
	if d.builder == nil {
		return fmt.Errorf("image %q not found locally and no builder configured", ref)
	}
	return d.builder(ctx, ref)
}

// Builder builds the given image reference. Set by the image package.
type Builder func(ctx context.Context, ref string) error

// SetBuilder wires a build function used by EnsureImage when an image is absent.
func (d *DockerCLI) SetBuilder(b Builder) { d.builder = b }

// Run executes the spec and returns the guest exit code. A non-zero guest exit
// is returned as (code, nil); only failures to launch docker itself return err.
func (d *DockerCLI) Run(ctx context.Context, spec RunSpec) (int, error) {
	args := BuildArgs(spec)
	cmd := exec.CommandContext(ctx, d.bin(), args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 1, fmt.Errorf("failed to run docker: %w", err)
}
