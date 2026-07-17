// Package cli wires the cobra command tree for the `sandbox-cli` binary.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/amitghadge/sandbox-cli/internal/config"
	"github.com/amitghadge/sandbox-cli/internal/image"
	"github.com/amitghadge/sandbox-cli/internal/runtime"
	"github.com/amitghadge/sandbox-cli/internal/sandbox"
)

// runFlags holds the persistent flag values shared by run/claude/codex.
type runFlags struct {
	project  string
	image    string
	workdir  string
	user     string
	mounts   []string
	env      []string
	envAllow []string
	tty      bool
	noTTY    bool
	config   string
	build    bool
	dryRun   bool

	// Auth persistence (agent wrappers only). persistName is the sandbox-owned
	// host state dir name (e.g. "claude"); persistSubdir is the agent's config
	// dir inside the container HOME (e.g. ".claude"). noPersistAuth opts out.
	persistName   string
	persistSubdir string
	noPersistAuth bool
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
	}

	// Persist agent auth in a dedicated, sandbox-owned host dir mounted at the
	// agent's config dir, so login survives the ephemeral container.
	if rf.persistName != "" && !rf.noPersistAuth {
		dir := config.AgentStateDir(rf.persistName)
		if dir != "" {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return nil, sandbox.Options{}, fmt.Errorf("creating auth persist dir %s: %w", dir, err)
			}
			opts.AuthPersistDir = dir
			opts.AuthPersistSubdir = rf.persistSubdir
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
