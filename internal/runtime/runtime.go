// Package runtime executes a normalized RunSpec inside a container backend.
//
// The MVP backend shells out to the `docker` CLI (docker_cli.go). Everything is
// hidden behind the Runtime interface so an SDK-based or remote/VM backend can
// be dropped in later without touching the callers.
package runtime

import "context"

// Mount is a single host->container bind mount.
type Mount struct {
	Source string // absolute host path
	Target string // absolute container path
	RO     bool   // read-only
}

// RunSpec is a fully-resolved, backend-agnostic container request. It carries no
// knowledge of config files or flags — sandbox.Session produces it. BuildArgs
// turns it into docker arguments and is exhaustively unit-tested, so this struct
// is the single choke point for the isolation invariants.
type RunSpec struct {
	Image    string
	Name     string            // container name (--name); enables `docker stats`, snapshots
	Workdir  string            // working dir inside the container (e.g. /workspace)
	Command  []string          // guest argv, e.g. ["claude", "--dangerously-skip-permissions"]
	TTY      bool              // allocate an interactive pty (-it)
	Remove   bool              // --rm: destroy container on exit
	Hostname string            // container hostname
	Home     string            // value for HOME inside the container (fake, ephemeral)
	User     string            // "" => image default; else "root", "sandbox", or uid:gid
	Network  string            // "" => docker default bridge; "none" => no network
	Env      map[string]string // explicit KEY=VALUE injected into the container
	EnvNames []string          // names forwarded from the host env (value read at exec time)
	Mounts   []Mount           // bind mounts (workspace + extras)

	// ShowMetrics enables the live resource bar (memory/CPU/elapsed) for
	// non-interactive runs. Requires Name to be set.
	ShowMetrics bool
	// ShowSummary prints a one-line peak-usage summary after the container exits.
	// Works for interactive runs too (sampled without drawing during the session).
	// Requires Name to be set.
	ShowSummary bool
}

// Runtime is a container execution backend.
type Runtime interface {
	// Available returns nil if the backend is usable (e.g. docker is installed
	// and the daemon is reachable).
	Available(ctx context.Context) error
	// EnsureImage makes sure the given image reference exists locally, building
	// or pulling it if necessary. forceBuild ignores any cached local image.
	EnsureImage(ctx context.Context, ref string, forceBuild bool) error
	// Run executes the spec and returns the guest's exit code.
	Run(ctx context.Context, spec RunSpec) (exitCode int, err error)
}
