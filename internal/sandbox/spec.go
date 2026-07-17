package sandbox

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/amitghadge/sandbox-cli/internal/config"
	"github.com/amitghadge/sandbox-cli/internal/runtime"
)

// Options are the per-invocation flag values collected by the CLI. Zero values
// mean "not set" and fall back to config.
type Options struct {
	Project     string   // --project: host dir for /workspace (default cwd)
	Image       string   // --image override
	Workdir     string   // --workdir override
	User        string   // --user override
	ExtraMounts []string // --mount host:container[:ro|rw]
	Env         []string // --env KEY=VALUE or bare KEY (forward host value)
	EnvAllow    []string // --env-allow NAME (forward host value if present)
	TTY         *bool    // --tty/--no-tty; nil => auto-detect
	Command     []string // guest argv

	// AuthPersistDir, when non-empty, is a host directory bind-mounted read-write
	// at <Home>/<AuthPersistSubdir> so an agent's credentials survive the
	// ephemeral container (log in once). Set by the claude/codex wrappers.
	AuthPersistDir    string
	AuthPersistSubdir string // e.g. ".claude"
}

// BuildSpec turns a merged config plus per-invocation options into a fully
// resolved runtime.RunSpec. It resolves and safety-checks the workspace, folds
// in config and flag mounts/env, and decides TTY allocation.
func BuildSpec(cfg config.Config, opts Options) (runtime.RunSpec, error) {
	ws, err := ResolveWorkspace(opts.Project)
	if err != nil {
		return runtime.RunSpec{}, err
	}

	image := cfg.Image
	if opts.Image != "" {
		image = opts.Image
	}
	workdir := cfg.Workdir
	if opts.Workdir != "" {
		workdir = opts.Workdir
	}
	user := cfg.User
	if opts.User != "" {
		user = opts.User
	}

	mounts := []runtime.Mount{WorkspaceMount(ws, workdirTargetOrDefault(cfg.Workdir))}

	// Config-declared mounts (host paths already resolved to absolute at load time).
	for _, m := range cfg.Mounts {
		mounts = append(mounts, runtime.Mount{
			Source: m.Host,
			Target: m.Container,
			RO:     m.Mode != "rw",
		})
	}
	// Flag mounts.
	for _, raw := range opts.ExtraMounts {
		m, err := parseMount(raw)
		if err != nil {
			return runtime.RunSpec{}, err
		}
		mounts = append(mounts, m)
	}

	// Auth-persistence mount: a sandbox-owned host dir at the agent's config dir
	// inside the (fake, ephemeral) HOME, so credentials survive --rm.
	if opts.AuthPersistDir != "" && opts.AuthPersistSubdir != "" {
		home := cfg.Home
		if home == "" {
			home = "/sandbox/home"
		}
		mounts = append(mounts, runtime.Mount{
			Source: opts.AuthPersistDir,
			Target: path.Join(home, opts.AuthPersistSubdir),
			RO:     false,
		})
	}

	env := map[string]string{}
	for k, v := range cfg.Env {
		env[k] = v
	}
	var envNames []string
	seen := map[string]bool{}
	addName := func(n string) {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		envNames = append(envNames, n)
	}

	// Config env_allow: forward host value only if present.
	for _, n := range cfg.EnvAllow {
		if _, ok := os.LookupEnv(n); ok {
			addName(n)
		}
	}
	for _, n := range opts.EnvAllow {
		if _, ok := os.LookupEnv(n); ok {
			addName(n)
		}
	}
	// --env: KEY=VALUE sets explicitly; bare KEY forwards host value.
	for _, e := range opts.Env {
		if k, v, ok := strings.Cut(e, "="); ok {
			env[k] = v
		} else {
			if _, ok := os.LookupEnv(e); ok {
				addName(e)
			}
		}
	}

	tty := detectTTY()
	if opts.TTY != nil {
		tty = *opts.TTY
	}

	return runtime.RunSpec{
		Image:    image,
		Workdir:  workdir,
		Command:  opts.Command,
		TTY:      tty,
		Remove:   true,
		Hostname: cfg.Hostname,
		Home:     cfg.Home,
		User:     user,
		Network:  cfg.NetworkArg(),
		Env:      env,
		EnvNames: envNames,
		Mounts:   mounts,
	}, nil
}

func workdirTargetOrDefault(workdir string) string {
	if workdir == "" {
		return "/workspace"
	}
	return workdir
}

// parseMount parses "host:container[:ro|rw]". Host ~ is expanded. Missing mode
// defaults to read-only (the conservative default).
func parseMount(raw string) (runtime.Mount, error) {
	parts := strings.Split(raw, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return runtime.Mount{}, fmt.Errorf("invalid --mount %q: want host:container[:ro|rw]", raw)
	}
	host := config.ExpandTilde(parts[0])
	container := parts[1]
	ro := true
	if len(parts) == 3 {
		switch parts[2] {
		case "ro":
			ro = true
		case "rw":
			ro = false
		default:
			return runtime.Mount{}, fmt.Errorf("invalid --mount mode %q in %q: want ro or rw", parts[2], raw)
		}
	}
	if host == "" || container == "" {
		return runtime.Mount{}, fmt.Errorf("invalid --mount %q: host and container must be non-empty", raw)
	}
	return runtime.Mount{Source: host, Target: container, RO: ro}, nil
}

// detectTTY reports whether stdin and stdout are both terminals.
func detectTTY() bool {
	return isTerminal(os.Stdin) && isTerminal(os.Stdout)
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
