// Package cli wires the cobra command tree for the `sandbox-cli` binary.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Aegmis/sandbox-cli/internal/config"
	"github.com/Aegmis/sandbox-cli/internal/image"
	"github.com/Aegmis/sandbox-cli/internal/runtime"
	"github.com/Aegmis/sandbox-cli/internal/sandbox"
	"github.com/Aegmis/sandbox-cli/internal/worktree"
)

// runFlags holds the persistent flag values shared by run/claude/codex.
type runFlags struct {
	project     string
	image       string
	workdir     string
	user        string
	mounts      []string
	env         []string
	envAllow    []string
	tty         bool
	noTTY       bool
	config      string
	build       bool
	dryRun      bool
	noMetrics   bool
	memory      string
	cpus        string
	noHardening bool
	allow       []string
	cache       bool
	secrets     []string
	worktree    string
	addHosts    []string
	hostGateway bool
	git         bool
	runtime     string

	// Auth persistence (agent wrappers only). persistName is the sandbox-owned
	// host state dir name (e.g. "claude") mounted as the agent's HOME.
	// noPersistAuth opts out.
	persistName   string
	noPersistAuth bool

	// noStatusline disables the sandbox mem/cpu status line for the claude wrapper.
	noStatusline bool

	// noSync (claude wrapper) opts out of read-write mounting the host's Claude
	// project history for this repo into the sandbox. Sharing it is the default,
	// so host sessions can be --resume'd from inside the container.
	noSync bool
}

// newSession loads config (with the flag override for the config path) and
// returns a wired Session plus the base Options derived from the flags. The
// guest command is filled in by each subcommand.
func newSession(rf *runFlags) (*sandbox.Session, sandbox.Options, error) {
	startDir, _ := os.Getwd()
	cfg, err := config.Load(startDir, rf.config)
	if err != nil {
		return nil, sandbox.Options{}, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, sandbox.Options{}, err
	}

	sess := sandbox.New(cfg)
	// Wire the lazy image builder into the docker backend.
	if d, ok := sess.Runtime.(*runtime.DockerCLI); ok {
		image.Register(d)
	}

	opts := sandbox.Options{
		Project:     rf.project,
		Image:       rf.image,
		Workdir:     rf.workdir,
		User:        rf.user,
		ExtraMounts: rf.mounts,
		Env:         rf.env,
		EnvAllow:    rf.envAllow,
		TTY:         ttyOverride(rf),
		NoMetrics:   rf.noMetrics,
		Memory:      rf.memory,
		CPUs:        rf.cpus,
		NoHardening: rf.noHardening,
		Allow:       rf.allow,
		Cache:       rf.cache,
		Secrets:     rf.secrets,
		AddHosts:    rf.addHosts,
		HostGateway: rf.hostGateway,
		GitIdentity: rf.git,
		Runtime:     rf.runtime,
	}

	// --worktree BRANCH: resolve (creating if needed) a git worktree for the
	// branch and run the sandbox in it, so parallel agents each get their own
	// branch/container without colliding. This overrides the project directory.
	if rf.worktree != "" {
		repoDir := rf.project
		if repoDir == "" {
			repoDir, _ = os.Getwd()
		}
		info, werr := worktree.Resolve(repoDir, rf.worktree)
		if werr != nil {
			return nil, sandbox.Options{}, werr
		}
		verb := "using"
		if info.Created {
			verb = "created"
		}
		fmt.Fprintf(os.Stderr, "sandbox-cli: %s worktree %q at %s\n", verb, info.Branch, info.Path)
		opts.Project = info.Path
	}

	// A git worktree's .git is a pointer file holding an absolute host path into
	// the parent repo, which lives outside the workspace. Without the parent .git
	// mounted at that same path, every git command inside the container fails with
	// "not a git repository" — the agent could edit files but never commit them.
	// Applies both to --worktree and to running from a worktree directly.
	projectDir := opts.Project
	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}
	if gitDir, ok := worktree.GitCommonDir(config.ExpandTilde(projectDir)); ok {
		opts.ExtraMounts = append(opts.ExtraMounts, gitDir+":"+gitDir+":rw")
	}

	// Persist agent login in a dedicated, sandbox-owned host dir mounted as the
	// agent's whole HOME, so login survives the ephemeral container.
	if rf.persistName != "" && !rf.noPersistAuth {
		dir := config.AgentStateDir(rf.persistName)
		if dir != "" {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return nil, sandbox.Options{}, fmt.Errorf("creating auth persist dir %s: %w", dir, err)
			}
			opts.AuthPersistDir = dir
		}
	}

	return sess, opts, nil
}

func ttyOverride(rf *runFlags) *bool {
	switch {
	case rf.noTTY:
		v := false
		return &v
	case rf.tty:
		v := true
		return &v
	default:
		return nil // auto-detect
	}
}

// addRunFlags registers the shared persistent flags on a command.
func addRunFlags(cmd *cobra.Command, rf *runFlags) {
	f := cmd.Flags()
	f.StringVarP(&rf.project, "project", "p", "", "host dir to mount at /workspace (default: cwd)")
	f.StringVarP(&rf.image, "image", "i", "", "override the container image")
	f.StringVarP(&rf.workdir, "workdir", "w", "", "working dir inside the container (default: /workspace)")
	f.StringVar(&rf.user, "user", "", "user inside the container (root|sandbox|uid:gid)")
	f.StringArrayVarP(&rf.mounts, "mount", "m", nil, "extra mount host:container[:ro|rw] (repeatable)")
	f.StringArrayVarP(&rf.env, "env", "e", nil, "KEY=VALUE, or bare KEY to forward host value (repeatable)")
	f.StringArrayVar(&rf.envAllow, "env-allow", nil, "host env var name to forward if present (repeatable)")
	f.BoolVar(&rf.tty, "tty", false, "force an interactive TTY")
	f.BoolVar(&rf.noTTY, "no-tty", false, "disable TTY allocation")
	f.StringVarP(&rf.config, "config", "c", "", "explicit config file path")
	f.BoolVar(&rf.build, "build", false, "force rebuild of the base image")
	f.BoolVar(&rf.dryRun, "dry-run", false, "print the docker command and exit")
	f.BoolVar(&rf.noMetrics, "no-metrics", false, "disable the live resource gauge (non-interactive runs)")
	f.StringVar(&rf.memory, "memory", "", "container memory limit, e.g. 2g (default: unlimited)")
	f.StringVar(&rf.cpus, "cpus", "", "container CPU limit, e.g. 1.5 (default: unlimited)")
	f.BoolVar(&rf.noHardening, "no-hardening", false, "disable default cap-drop/no-new-privileges/pids-limit (debug)")
	f.StringArrayVar(&rf.allow, "allow", nil, "enable the egress allowlist and permit DOMAIN (repeatable; baseline registries always allowed)")
	f.BoolVar(&rf.cache, "cache", false, "persist package-manager caches (npm/pip/cargo/go) in named volumes across runs")
	f.StringArrayVar(&rf.secrets, "secret", nil, "brokered credential NAME=file:PATH|cmd:COMMAND|env:VAR, resolved at run time and kept off the argv (repeatable)")
	f.StringVar(&rf.worktree, "worktree", "", "run in a git worktree for BRANCH (created if absent), for parallel per-branch agents")
	f.StringArrayVar(&rf.addHosts, "add-host", nil, "extra HOST:IP mapping passed to docker (repeatable)")
	f.BoolVar(&rf.hostGateway, "host-gateway", false, "map host.docker.internal to the host so the agent can reach host MCP servers (Linux)")
	f.BoolVar(&rf.git, "git", false, "forward host git identity and trust the workspace so git commits just work in-container")
	f.StringVar(&rf.runtime, "runtime", "", "OCI runtime for stronger isolation, e.g. kata-runtime (microVM) or runsc (gVisor); must be registered with docker")

	// Flags before -- are ours; everything after -- is the guest command verbatim.
	f.SetInterspersed(false)
}

// NewRootCmd builds the top-level command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "sandbox-cli",
		Short: "Run AI coding agents in a disposable, isolated container",
		Long: "sandbox-cli runs a command (or an AI coding agent) inside a throwaway Docker\n" +
			"container where only the chosen project is mounted at /workspace and HOME is\n" +
			"a fake, ephemeral directory. A mistaken `rm -rf ~` or an injected command\n" +
			"cannot touch the rest of your machine.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newRunCmd(),
		newClaudeCmd(),
		newCodexCmd(),
		newInitCmd(),
		newConfigCmd(),
		newStatsCmd(),
		newWorktreeCmd(),
		newVersionCmd(),
	)
	return root
}

// Execute runs the CLI and returns a process exit code.
func Execute() int {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "sandbox-cli: "+err.Error())
		return 1
	}
	return exitCode
}

// exitCode carries the guest process exit code out of a subcommand's RunE so the
// process can mirror it. Defaults to 0.
var exitCode int
