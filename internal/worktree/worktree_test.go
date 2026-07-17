package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeBranch(t *testing.T) {
	cases := map[string]string{
		"main":            "main",
		"feature/login":   "feature-login",
		"user/fix.bug_v2": "user-fix.bug_v2",
		"///weird//":      "weird",
		"a  b":            "a-b",
	}
	for in, want := range cases {
		if got := sanitizeBranch(in); got != want {
			t.Errorf("sanitizeBranch(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWorktreePath_StableAndNamespaced(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p1 := worktreePath("/repos/alpha", "feature/x")
	// Deterministic.
	if p1 != worktreePath("/repos/alpha", "feature/x") {
		t.Error("worktreePath not deterministic")
	}
	// Branch is sanitized into the final segment.
	if filepath.Base(p1) != "feature-x" {
		t.Errorf("leaf = %q, want feature-x", filepath.Base(p1))
	}
	// Same-named repos at different paths get different bases (hash namespacing).
	if worktreeBase("/repos/alpha") == worktreeBase("/other/alpha") {
		t.Error("expected different bases for same-named repos at different paths")
	}
}

func TestResolveAndList_RealGit(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Build a throwaway repo with one commit.
	repo := t.TempDir()
	runOrSkip(t, git, repo, "init", "-q")
	runOrSkip(t, git, repo, "config", "user.email", "t@example.com")
	runOrSkip(t, git, repo, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runOrSkip(t, git, repo, "add", ".")
	runOrSkip(t, git, repo, "commit", "-qm", "init")

	// New branch: created from HEAD.
	info, err := Resolve(repo, "feature/x")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Created {
		t.Error("expected Created=true for a new worktree")
	}
	if !isDir(info.Path) {
		t.Errorf("worktree dir not created: %s", info.Path)
	}
	if _, err := os.Stat(filepath.Join(info.Path, "README")); err != nil {
		t.Errorf("worktree missing repo content: %v", err)
	}

	// Idempotent reuse.
	again, err := Resolve(repo, "feature/x")
	if err != nil {
		t.Fatal(err)
	}
	if again.Created {
		t.Error("expected reuse (Created=false) on the second Resolve")
	}
	if again.Path != info.Path {
		t.Errorf("reuse path %q != %q", again.Path, info.Path)
	}

	// List shows it.
	infos, err := List(repo)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, wt := range infos {
		if wt.Branch == "feature/x" {
			found = true
		}
	}
	if !found {
		t.Errorf("List missing feature/x: %+v", infos)
	}

	// Remove it.
	if err := Remove(repo, "feature/x"); err != nil {
		t.Fatal(err)
	}
	if isDir(info.Path) {
		t.Errorf("worktree dir still present after Remove: %s", info.Path)
	}
}

func TestResolve_NotAGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	if _, err := Resolve(dir, "x"); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("expected a not-a-git-repo error, got %v", err)
	}
}

func runOrSkip(t *testing.T, git, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(git, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git %v failed (%v): %s", args, err, out)
	}
}
