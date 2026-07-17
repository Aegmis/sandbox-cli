package cli

import "github.com/spf13/cobra"

// claudeEnvAllow is the suggested (opt-in) set of host env vars forwarded to a
// Claude Code session, applied only if present in the host environment. Nothing
// else about the host crosses the boundary.
var claudeEnvAllow = []string{
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_AUTH_TOKEN",
	"ANTHROPIC_BASE_URL",
	"CLAUDE_CODE_USE_BEDROCK",
	"CLAUDE_CODE_USE_VERTEX",
}

func newClaudeCmd() *cobra.Command {
	rf := &runFlags{}
	cmd := &cobra.Command{
		Use:   "claude [sandbox-flags --] [claude-args...]",
		Short: "Run Claude Code inside the sandbox",
		Long: "Runs `claude` inside the sandbox. Everything you pass is forwarded to\n" +
			"claude, so `sandbox-cli claude --dangerously-skip-permissions` just works. Sandbox\n" +
			"options (leading --flags below, or before a `--` separator) are consumed first.\n\n" +
			"Your Claude login is persisted by default in a sandbox-owned directory\n" +
			"(~/.config/sandbox/agents/claude, separate from your host ~/.claude), so you\n" +
			"log in once. Use --no-persist-auth for a throwaway session.\n\n" +
			"Forwards ANTHROPIC_API_KEY and related variables from your host environment\n" +
			"only if they are set. No other host files are mounted unless you pass --mount.",
		Example: "  sandbox-cli claude\n" +
			"  sandbox-cli claude --dangerously-skip-permissions\n" +
			"  sandbox-cli claude --project ~/app -- --dangerously-skip-permissions",
		// Forward unknown agent flags instead of rejecting them; sandbox flags are
		// parsed manually from the pre-`--` portion in runWrapper.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWrapper(cmd, rf, args, "claude", claudeEnvAllow)
		},
	}
	addRunFlags(cmd, rf)
	// Persist Claude's login in a sandbox-owned host dir (~/.config/sandbox/
	// agents/claude) mounted at ~/.claude, so you log in once. Opt out with
	// --no-persist-auth.
	rf.persistName = "claude"
	rf.persistSubdir = ".claude"
	cmd.Flags().BoolVar(&rf.noPersistAuth, "no-persist-auth", false, "do not persist the agent login across runs")
	return cmd
}
