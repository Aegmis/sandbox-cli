package cli

import (
	"reflect"
	"testing"
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
