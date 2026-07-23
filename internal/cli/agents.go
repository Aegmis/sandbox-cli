package cli

import "github.com/spf13/cobra"

// Shared plumbing for the agent wrappers (claude/codex/gemini/opencode).

// agentAnnotation keys the cobra annotation carrying an adapter's agent name —
// the same string that becomes its persisted-HOME directory. It exists so the
// wiring can be inspected (by tests, and by anything that wants to enumerate the
// adapters) without every wrapper having to expose its runFlags.
const agentAnnotation = "sandbox-cli/agent"

// finishAgentCmd applies the wiring every agent adapter shares: the common
// sandbox flag set, a sandbox-owned host dir named after the agent mounted as
// its whole HOME so the login survives the throwaway container, and the opt-out
// for it. Wrappers with extra behaviour (claude) add their own flags on top.
func finishAgentCmd(cmd *cobra.Command, rf *runFlags, agent string) *cobra.Command {
	addRunFlags(cmd, rf)
	rf.persistName = agent
	cmd.Flags().BoolVar(&rf.noPersistAuth, "no-persist-auth", false, "do not persist the agent login across runs")
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[agentAnnotation] = agent
	return cmd
}

// npmAgentBootstrap builds the container command for an agent distributed as an
// npm package. The baked copy in the base image is used when it is there; when
// it is not — an older base image, or a package whose install was best-effort at
// build time — the agent is installed into the persisted HOME (~/.local, already
// ahead on PATH) on first run and reused from there afterwards.
//
// This is deliberately weaker than the claude wrapper's bootstrap, which always
// prefers the self-updating HOME install: npm-packaged agents are pinned by the
// image tag, and the image tag changes whenever the embedded Dockerfile does, so
// updating them is an image rebuild rather than a per-run download.
//
// The trailing bin is sh's argv[0] for the script, so "$@" is exactly the guest
// args appended by runWrapper.
func npmAgentBootstrap(bin, pkg string) []string {
	script := `export PATH="$HOME/.local/bin:$PATH"
if ! command -v ` + bin + ` >/dev/null 2>&1; then
  npm install -g --prefix "$HOME/.local" ` + pkg + ` >/dev/null 2>&1 || true
fi
exec ` + bin + ` "$@"`
	return []string{"sh", "-c", script, bin}
}
