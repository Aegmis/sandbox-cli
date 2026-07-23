package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSplitWrapperArgs(t *testing.T) {
	cases := []struct {
		name      string
		in        []string
		wantFlags []string
		wantGuest []string
	}{
		{
			name:      "bare agent flag passes through (the reported bug)",
			in:        []string{"--dangerously-skip-permissions"},
			wantFlags: []string{},
			wantGuest: []string{"--dangerously-skip-permissions"},
		},
		{
			name:      "no args",
			in:        []string{},
			wantFlags: []string{},
			wantGuest: []string{},
		},
		{
			name:      "colliding short flag goes to agent, not sandbox",
			in:        []string{"-p", "do the thing"},
			wantFlags: []string{},
			wantGuest: []string{"-p", "do the thing"},
		},
		{
			name:      "leading sandbox long-flags consumed, rest to agent (no -- needed)",
			in:        []string{"--no-persist-auth", "--dangerously-skip-permissions"},
			wantFlags: []string{"--no-persist-auth"},
			wantGuest: []string{"--dangerously-skip-permissions"},
		},
		{
			name:      "sandbox value flag consumes its value",
			in:        []string{"--project", "/x", "--dangerously-skip-permissions"},
			wantFlags: []string{"--project", "/x"},
			wantGuest: []string{"--dangerously-skip-permissions"},
		},
		{
			name:      "dry-run alone (natural, no separator)",
			in:        []string{"--dry-run"},
			wantFlags: []string{"--dry-run"},
			wantGuest: []string{},
		},
		{
			name:      "explicit -- forces boundary and is dropped",
			in:        []string{"--project", "/x", "--", "--dangerously-skip-permissions", "--model", "opus"},
			wantFlags: []string{"--project", "/x"},
			wantGuest: []string{"--dangerously-skip-permissions", "--model", "opus"},
		},
		{
			name:      "unknown long flag (agent's) ends sandbox portion",
			in:        []string{"--model", "opus"},
			wantFlags: []string{},
			wantGuest: []string{"--model", "opus"},
		},
		{
			name:      "--flag=value form",
			in:        []string{"--project=/x", "--dangerously-skip-permissions"},
			wantFlags: []string{"--project=/x"},
			wantGuest: []string{"--dangerously-skip-permissions"},
		},
	}
	cmd := newClaudeCmd() // real command so Flags() knows sandbox's flag set
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotFlags, gotGuest := splitWrapperArgs(cmd, c.in)
			if !reflect.DeepEqual(gotFlags, c.wantFlags) {
				t.Errorf("flags = %#v, want %#v", gotFlags, c.wantFlags)
			}
			if !reflect.DeepEqual(gotGuest, c.wantGuest) {
				t.Errorf("guest = %#v, want %#v", gotGuest, c.wantGuest)
			}
		})
	}
}

// TestClaudeWrapperForwardsUnknownFlags exercises the full command so a
// regression in DisableFlagParsing would be caught: the wrapper must not reject
// an unknown agent flag at parse time.
func TestClaudeWrapperParsesWithoutError(t *testing.T) {
	cmd := newClaudeCmd()
	// DisableFlagParsing must be set, otherwise cobra rejects --dangerously-skip-permissions.
	if !cmd.DisableFlagParsing {
		t.Fatal("claude wrapper must set DisableFlagParsing to forward agent flags")
	}
}

// TestAgentWrappersShareTheContract pins the properties every agent adapter must
// have, so a new one added by copying an existing file can't quietly drop them:
// unknown agent flags are forwarded rather than rejected, the shared sandbox
// flag set is present, and the login persists in a sandbox-owned host dir of its
// own with an opt-out. Distinct persist names matter most — two adapters sharing
// one would cross their logins into a single directory.
func TestAgentWrappersShareTheContract(t *testing.T) {
	agents := map[string]bool{}
	for _, ctor := range []func() *cobra.Command{
		newClaudeCmd, newCodexCmd, newGeminiCmd, newOpencodeCmd,
	} {
		cmd := ctor()
		name := strings.Fields(cmd.Use)[0]
		t.Run(name, func(t *testing.T) {
			if !cmd.DisableFlagParsing {
				t.Error("must set DisableFlagParsing to forward agent flags")
			}
			for _, f := range []string{"project", "worktree", "dry-run", "no-persist-auth"} {
				if cmd.Flags().Lookup(f) == nil {
					t.Errorf("missing sandbox flag --%s", f)
				}
			}
			// Set by finishAgentCmd from the same string it assigns to
			// rf.persistName, which newSession turns into the persisted HOME.
			agent := cmd.Annotations[agentAnnotation]
			if agent == "" {
				t.Fatal("no agent annotation: the login would not persist across runs")
			}
			if agents[agent] {
				t.Errorf("agent name %q is used by more than one wrapper", agent)
			}
			agents[agent] = true
		})
	}
}

// TestNpmAgentBootstrap checks the shape the guest argv relies on: a shell
// script whose argv[0] is the agent, so runWrapper's forwarded args arrive as
// "$@" and the agent is exec'd (not left as a child of sh, which would swallow
// signals and the exit code).
func TestNpmAgentBootstrap(t *testing.T) {
	got := npmAgentBootstrap("gemini", "@google/gemini-cli")
	if len(got) != 4 || got[0] != "sh" || got[1] != "-c" || got[3] != "gemini" {
		t.Fatalf("bootstrap argv = %#v, want [sh -c <script> gemini]", got)
	}
	for _, want := range []string{`exec gemini "$@"`, "@google/gemini-cli", `$HOME/.local`} {
		if !strings.Contains(got[2], want) {
			t.Errorf("script does not contain %q:\n%s", want, got[2])
		}
	}
}

func TestClaudeProjectBucket(t *testing.T) {
	cases := map[string]string{
		"/Users/amitghadge/project/sandbox-cli": "-Users-amitghadge-project-sandbox-cli",
		"/workspace":                            "-workspace",
		"/Users/x/.agent/ai":                    "-Users-x--agent-ai", // '/.' -> '--'
	}
	for in, want := range cases {
		if got := claudeProjectBucket(in); got != want {
			t.Errorf("claudeProjectBucket(%q) = %q, want %q", in, got, want)
		}
	}
}
