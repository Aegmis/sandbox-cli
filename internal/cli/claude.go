package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/amitghadge/sandbox-cli/internal/config"
)

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

// claudeBootstrap ensures a self-updating Claude install exists in the persisted
// HOME (~/.local/bin, installed via the native installer on first run) and execs
// it. The baked npm copy in /usr/local/bin is the offline fallback. Because the
// persisted install is user-writable, Claude Code keeps itself up to date across
// runs — the baked copy could not (root-owned).
const claudeBootstrap = `export PATH="$HOME/.local/bin:$PATH"
if [ ! -x "$HOME/.local/bin/claude" ]; then
  command -v curl >/dev/null 2>&1 && curl -fsSL https://claude.ai/install.sh | bash >/dev/null 2>&1 || true
fi
exec claude "$@"`

// claudeStatuslineSettings is the managed-settings.json (highest precedence, does
// not touch the user's own settings) that points Claude Code's status line at the
// baked cgroup mem/cpu script.
const claudeStatuslineSettings = `{"statusLine":{"type":"command","command":"/usr/local/bin/sandbox-statusline","padding":0,"refreshInterval":3}}
`

func newClaudeCmd() *cobra.Command {
	rf := &runFlags{}
	cmd := &cobra.Command{
		Use:   "claude [sandbox-flags --] [claude-args...]",
		Short: "Run Claude Code inside the sandbox",
		Long: "Runs `claude` inside the sandbox. Everything you pass is forwarded to\n" +
			"claude, so `sandbox-cli claude --dangerously-skip-permissions` just works. Sandbox\n" +
			"options (leading --flags below, or before a `--` separator) are consumed first.\n\n" +
			"Claude Code is installed into the persisted HOME on first run and self-updates\n" +
			"from there, so it stays current (the baked image copy is an offline fallback).\n" +
			"A status line showing the container's memory/CPU is added to the Claude UI;\n" +
			"disable it with --no-statusline.\n\n" +
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
			agentCmd := []string{"sh", "-c", claudeBootstrap, "claude"}
			afterParse := func() error {
				if rf.noStatusline {
					return nil
				}
				p, err := ensureClaudeStatuslineSettings()
				if err != nil {
					// Non-fatal: the status line is a nicety, not core function.
					fmt.Fprintln(os.Stderr, "sandbox-cli: status line disabled: "+err.Error())
					return nil
				}
				rf.mounts = append(rf.mounts, p+":/etc/claude-code/managed-settings.json:ro")
				return nil
			}
			return runWrapper(cmd, rf, args, agentCmd, claudeEnvAllow, afterParse)
		},
	}
	addRunFlags(cmd, rf)
	// Persist Claude's login in a sandbox-owned host dir (~/.config/sandbox/
	// agents/claude) mounted as the container HOME, so you log in once. Opt out
	// with --no-persist-auth.
	rf.persistName = "claude"
	cmd.Flags().BoolVar(&rf.noPersistAuth, "no-persist-auth", false, "do not persist the agent login across runs")
	cmd.Flags().BoolVar(&rf.noStatusline, "no-statusline", false, "don't add the sandbox memory/CPU status line to Claude")
	return cmd
}

// ensureClaudeStatuslineSettings writes the managed-settings.json to a sandbox-
// owned host path and returns it, for read-only mounting into the container.
func ensureClaudeStatuslineSettings() (string, error) {
	root := config.ConfigRoot()
	if root == "" {
		return "", fmt.Errorf("cannot determine config dir")
	}
	dir := filepath.Join(root, "managed")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "claude-managed-settings.json")
	if err := os.WriteFile(p, []byte(claudeStatuslineSettings), 0o644); err != nil {
		return "", err
	}
	return p, nil
}
