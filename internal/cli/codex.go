package cli

import "github.com/spf13/cobra"

// codexEnvAllow is the suggested (opt-in) set of host env vars forwarded to a
// Codex CLI session, applied only if present in the host environment.
var codexEnvAllow = []string{
	"OPENAI_API_KEY",
	"OPENAI_BASE_URL",
	"CODEX_HOME",
}

func newCodexCmd() *cobra.Command {
	rf := &runFlags{}
	cmd := &cobra.Command{
		Use:   "codex [sandbox-flags --] [codex-args...]",
		Short: "Run Codex CLI inside the sandbox",
		Long: "Runs `codex` inside the sandbox. Everything you pass is forwarded to codex.\n" +
			"To set sandbox options, put them before a `--` separator, e.g.\n" +
			"`sandbox codex --project ~/app -- exec 'run the tests'`.\n\n" +
			"Forwards OPENAI_API_KEY and related variables from your host environment\n" +
			"only if they are set. No host files or credentials are mounted unless you\n" +
			"pass --mount explicitly.",
		Example: "  sandbox codex\n" +
			"  sandbox codex exec 'run the tests'\n" +
			"  sandbox codex --project ~/app -- exec 'run the tests'",
		// Forward unknown agent flags instead of rejecting them; sandbox flags are
		// parsed manually from the pre-`--` portion in runWrapper.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWrapper(cmd, rf, args, "codex", codexEnvAllow)
		},
	}
	addRunFlags(cmd, rf)
	// Persist Codex's login in a sandbox-owned host dir (~/.config/sandbox/
	// agents/codex) mounted at ~/.codex. Opt out with --no-persist-auth.
	rf.persistName = "codex"
	rf.persistSubdir = ".codex"
	cmd.Flags().BoolVar(&rf.noPersistAuth, "no-persist-auth", false, "do not persist the agent login across runs")
	return cmd
}
