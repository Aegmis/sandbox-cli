package cli

import "github.com/spf13/cobra"

// opencodeEnvAllow is the suggested (opt-in) set of host env vars forwarded to
// an OpenCode session, applied only if present in the host environment.
// OpenCode is provider-agnostic, so the list spans the providers it can drive
// rather than naming a single vendor; each is forwarded only if you have it set.
var opencodeEnvAllow = []string{
	"ANTHROPIC_API_KEY",
	"OPENAI_API_KEY",
	"GEMINI_API_KEY",
	"GROQ_API_KEY",
	"OPENROUTER_API_KEY",
	"OPENCODE_CONFIG",
	"OPENCODE_DISABLE_AUTOUPDATE",
}

func newOpencodeCmd() *cobra.Command {
	rf := &runFlags{}
	cmd := &cobra.Command{
		Use:   "opencode [sandbox-flags --] [opencode-args...]",
		Short: "Run OpenCode inside the sandbox",
		Long: "Runs `opencode` inside the sandbox. Everything you pass is forwarded to\n" +
			"opencode. Sandbox options (leading --flags below, or before a `--` separator)\n" +
			"are consumed first.\n\n" +
			"Your OpenCode login is persisted by default in a sandbox-owned directory\n" +
			"(~/.config/sandbox/agents/opencode, separate from your host OpenCode config),\n" +
			"so `opencode auth login` survives the throwaway container. Use\n" +
			"--no-persist-auth for a session that keeps nothing.\n\n" +
			"OpenCode drives several providers, so the API keys of each are forwarded from\n" +
			"your host environment only if they are set. No other host files are mounted\n" +
			"unless you pass --mount.",
		Example: "  sandbox-cli opencode\n" +
			"  sandbox-cli opencode run 'run the tests'\n" +
			"  sandbox-cli opencode --project ~/app -- run 'fix the failing test'",
		// Forward unknown agent flags instead of rejecting them; sandbox flags are
		// parsed manually from the pre-`--` portion in runWrapper.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			agentCmd := npmAgentBootstrap("opencode", "opencode-ai")
			return runWrapper(cmd, rf, args, agentCmd, opencodeEnvAllow, nil)
		},
	}
	// Persists OpenCode's login in a sandbox-owned host dir (~/.config/sandbox/
	// agents/opencode) mounted as the container HOME. Opt out with
	// --no-persist-auth.
	return finishAgentCmd(cmd, rf, "opencode")
}
