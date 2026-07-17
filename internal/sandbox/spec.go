package sandbox

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

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
	NoMetrics   bool     // disable the live resource gauge
	Memory      string   // --memory: container memory limit (e.g. "2g"); "" => config/unlimited
	CPUs        string   // --cpus: container CPU limit (e.g. "1.5"); "" => config/unlimited
	NoHardening bool     // --no-hardening: drop cap-drop/no-new-privileges/pids-limit (debug escape hatch)
	Allow       []string // --allow DOMAIN: enable the egress allowlist and permit these domains (repeatable)
	Command     []string // guest argv

	// AuthPersistDir, when non-empty, is a host directory bind-mounted read-write
	// as the agent's whole HOME so its login/config survives the ephemeral
	// container (log in once). Set by the claude/codex wrappers.
	AuthPersistDir string
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

	// Auth-persistence mount: a sandbox-owned host dir mounted as the whole
	// agent HOME, so everything the agent writes there — ~/.claude.json (the
	// "onboarding done" flag + account), ~/.claude/.credentials.json, ~/.codex —
	// survives the ephemeral --rm container and you log in once. Mounting the
	// whole home (not just ~/.claude) is required because config files are
	// written via atomic rename, which a single-file bind mount cannot persist.
	if opts.AuthPersistDir != "" {
		home := cfg.Home
		if home == "" {
			home = "/sandbox/home"
		}
		mounts = append(mounts, runtime.Mount{
			Source: opts.AuthPersistDir,
			Target: home,
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

	// Resolve the hardening policy from config, then apply flag overrides.
	// --no-hardening is a debug escape hatch that reverts to the historical
	// "no cap-drop / no-new-privileges / no pids cap" behavior; it deliberately
	// does not touch the opt-in Memory/CPU limits.
	sec := cfg.Security
	noNewPriv := sec.NoNewPriv()
	capDrop := sec.CapDrop
	capAdd := sec.CapAdd
	pids := sec.Pids()
	if opts.NoHardening {
		noNewPriv = false
		capDrop = nil
		capAdd = nil
		pids = 0
	}
	memory := sec.Memory
	if opts.Memory != "" {
		memory = opts.Memory
	}
	cpus := sec.CPUs
	if opts.CPUs != "" {
		cpus = opts.CPUs
	}

	// Egress allowlist. Config `network.mode: allowlist` contributes the baseline
	// plus configured domains; `--allow DOMAIN` adds domains and, on its own,
	// switches the allowlist on. When active, the container needs a default-deny
	// egress firewall: that is programmed in-container at startup by the
	// sandbox-firewall entrypoint, which requires running as root with NET_ADMIN
	// and then drops back to the intended user (SANDBOX_RUN_AS) before the agent
	// runs. Allowlist implies bridge networking, so it overrides `none`.
	network := cfg.NetworkArg()
	egress := cfg.Network.EgressDomains()
	if len(opts.Allow) > 0 {
		if egress == nil {
			egress = config.BaselineEgress()
		}
		egress = config.DedupeDomains(append(egress, opts.Allow...))
	}
	dockerUser := user
	entrypoint := ""
	if len(egress) > 0 {
		runAs := user
		if runAs == "" {
			runAs = "sandbox"
		}
		env["SANDBOX_EGRESS_ALLOW"] = strings.Join(egress, ",")
		env["SANDBOX_RUN_AS"] = runAs
		capAdd = append(capAdd, "NET_ADMIN", "NET_RAW")
		dockerUser = "root"
		entrypoint = "/usr/local/bin/sandbox-firewall"
		network = "" // allowlist requires bridge networking, not "none"
	}

	tty := detectTTY()
	if opts.TTY != nil {
		tty = *opts.TTY
	}

	// Metrics require a terminal to report to. The live gauge is drawn only for
	// non-interactive runs (an interactive agent TUI owns the terminal); the
	// post-run summary is printed for all runs, including interactive ones, since
	// it only appears after the session ends.
	metricsOn := !opts.NoMetrics && isTerminal(os.Stderr)
	showMetrics := metricsOn && !tty
	showSummary := metricsOn

	return runtime.RunSpec{
		Image:    image,
		Name:     containerName(),
		Workdir:  workdir,
		Command:  opts.Command,
		TTY:      tty,
		Remove:   true,
		Hostname: cfg.Hostname,
		Home:     cfg.Home,
		User:     dockerUser,
		Network:  network,
		Env:      env,
		EnvNames: envNames,
		Mounts:   mounts,

		Entrypoint: entrypoint,

		NoNewPrivileges: noNewPriv,
		Seccomp:         sec.Seccomp,
		CapDrop:         capDrop,
		CapAdd:          capAdd,
		PidsLimit:       pids,
		Memory:          memory,
		CPUs:            cpus,

		ShowMetrics: showMetrics,
		ShowSummary: showSummary,
	}, nil
}

// containerName returns a unique, docker-valid container name. A stable prefix
// makes sandbox containers easy to spot (and, later, filter for `stats`).
func containerName() string {
	return "sandbox-" + strconv.FormatInt(time.Now().UnixNano(), 36)
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
