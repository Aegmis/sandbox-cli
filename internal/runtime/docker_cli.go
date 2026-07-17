package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/amitghadge/sandbox-cli/internal/metrics"
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

	if spec.Name != "" && spec.ShowMetrics {
		return d.runWithLiveGauge(cmd, spec)
	}
	if spec.Name != "" && spec.ShowSummary {
		return d.runWithSummary(cmd, spec)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return exitCodeOf(cmd.Run())
}

// runWithLiveGauge forwards the container's output through a sticky footer that
// shows a live resource gauge, erases the gauge on exit, and prints a summary.
func (d *DockerCLI) runWithLiveGauge(cmd *exec.Cmd, spec RunSpec) (int, error) {
	footer := metrics.NewTermFooter(os.Stderr)
	outR, outW := io.Pipe()
	errR, errW := io.Pipe()
	cmd.Stdout = outW
	cmd.Stderr = errW

	var pumps sync.WaitGroup
	pumps.Add(2)
	go pump(&pumps, outR, os.Stdout, footer)
	go pump(&pumps, errR, os.Stderr, footer)

	meter := metrics.NewMeter(d.bin(), spec.Name, footer)
	meter.Start()

	runErr := cmd.Run()

	// Drain forwarders, then stop the gauge and erase it.
	outW.Close()
	errW.Close()
	pumps.Wait()
	meter.Stop()

	printSummary(spec, meter)
	return exitCodeOf(runErr)
}

// runWithSummary keeps direct stdio (so an interactive TTY works) while sampling
// resource usage silently, then prints a one-line summary after the run.
func (d *DockerCLI) runWithSummary(cmd *exec.Cmd, spec RunSpec) (int, error) {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	meter := metrics.NewMeter(d.bin(), spec.Name, nil) // nil footer => silent
	meter.Start()
	runErr := cmd.Run()
	meter.Stop()

	printSummary(spec, meter)
	return exitCodeOf(runErr)
}

// printSummary writes the meter's summary line to stderr, if any was captured.
func printSummary(spec RunSpec, meter *metrics.Meter) {
	if !spec.ShowSummary {
		return
	}
	if s := meter.Summary(); s != "" {
		fmt.Fprintln(os.Stderr, "\033[2m"+s+"\033[0m")
	}
}

// pump forwards src to dst through the footer until src is exhausted.
func pump(wg *sync.WaitGroup, src io.Reader, dst *os.File, footer *metrics.TermFooter) {
	defer wg.Done()
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			footer.Print(dst, buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func exitCodeOf(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 1, fmt.Errorf("failed to run docker: %w", err)
}
