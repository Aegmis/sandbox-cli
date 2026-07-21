// Package worktree makes "one sandbox per git branch" a one-liner. It manages
// git worktrees in a sandbox-owned location (under the config dir, so the user's
// project directory stays clean) so several agents can run in parallel, each on
// its own branch in its own container, without colliding — then reviewed with a
// normal `git checkout <branch>`.
package worktree

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aegmis/sandbox-cli/internal/config"
)

// gitBin is the git executable; a variable so tests can stub it if needed.
var gitBin = "git"

// Info describes a resolved worktree.
type Info struct {
	Branch  string
	Path    string
	Created bool // true when Resolve created it this call (vs. reused)
}

// RepoRoot returns the top-level directory of the git repository containing dir.
func RepoRoot(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("%q is not a git repository (required for --worktree): %w", dir, err)
	}
	return strings.TrimSpace(out), nil
}

// Resolve ensures a git worktree for branch exists in the repo containing dir and
// returns it. If the branch does not exist it is created from the current HEAD.
// An existing sandbox worktree for the branch is reused.
func Resolve(dir, branch string) (Info, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return Info{}, fmt.Errorf("worktree: empty branch name")
	}
	root, err := RepoRoot(dir)
	if err != nil {
		return Info{}, err
	}
	path := worktreePath(root, branch)
	info := Info{Branch: branch, Path: path}

	// Reuse an existing worktree directory (git tracks it; re-adding would error).
	if isDir(path) {
		return info, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Info{}, fmt.Errorf("worktree: preparing directory: %w", err)
	}

	args := []string{"worktree", "add"}
	if branchExists(root, branch) {
		args = append(args, path, branch)
	} else {
		args = append(args, "-b", branch, path)
	}
	if _, err := runGit(root, args...); err != nil {
		return Info{}, fmt.Errorf("creating worktree for branch %q: %w", branch, err)
	}
	info.Created = true
	return info, nil
}

// List returns the sandbox-managed worktrees for the repo containing dir (those
// living under the managed base directory).
func List(dir string) ([]Info, error) {
	root, err := RepoRoot(dir)
	if err != nil {
		return nil, err
	}
	out, err := runGit(root, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	// Compare on symlink-resolved paths: `git worktree list` reports resolved
	// paths (e.g. /private/var/... on macOS) while worktreeBase is the logical
	// path (/var/...), so a raw prefix check would drop every managed worktree.
	base := resolveSymlinks(worktreeBase(root))
	var infos []Info
	var cur Info
	flush := func() {
		if cur.Path != "" && strings.HasPrefix(resolveSymlinks(cur.Path), base) {
			infos = append(infos, cur)
		}
		cur = Info{}
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}
	flush()
	return infos, nil
}

// Remove deletes the sandbox worktree for branch (git worktree remove).
func Remove(dir, branch string) error {
	root, err := RepoRoot(dir)
	if err != nil {
		return err
	}
	path := worktreePath(root, branch)
	if _, err := runGit(root, "worktree", "remove", path); err != nil {
		return fmt.Errorf("removing worktree for branch %q: %w", branch, err)
	}
	return nil
}

func branchExists(root, branch string) bool {
	_, err := runGit(root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// worktreeBase is the managed directory holding all worktrees for one repo,
// namespaced by repo name + a short hash of its absolute path so identically
// named repos never collide.
func worktreeBase(repoRoot string) string {
	root := config.ConfigRoot()
	if root == "" {
		root = filepath.Join(os.TempDir(), "sandbox")
	}
	sum := sha256.Sum256([]byte(repoRoot))
	id := filepath.Base(repoRoot) + "-" + hex.EncodeToString(sum[:])[:8]
	return filepath.Join(root, "worktrees", id)
}

// worktreePath is the managed path for a single branch's worktree.
func worktreePath(repoRoot, branch string) string {
	return filepath.Join(worktreeBase(repoRoot), sanitizeBranch(branch))
}

// sanitizeBranch turns a branch name into a safe single path segment
// (e.g. "feature/login" -> "feature-login").
func sanitizeBranch(b string) string {
	var sb strings.Builder
	prevDash := true // suppress leading separators
	for _, r := range b {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_':
			sb.WriteRune(r)
			prevDash = false
		case !prevDash:
			sb.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(sb.String(), "-")
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// resolveSymlinks returns the symlink-resolved path, falling back to the cleaned
// input when it can't be resolved (e.g. the path does not exist yet).
func resolveSymlinks(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command(gitBin, args...)
	cmd.Dir = dir
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errb.String()); msg != "" {
			return "", fmt.Errorf("%s: %w", msg, err)
		}
		return "", err
	}
	return out.String(), nil
}
