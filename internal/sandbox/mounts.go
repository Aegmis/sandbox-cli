package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amitghadge/sandbox-cli/internal/config"
	"github.com/amitghadge/sandbox-cli/internal/runtime"
)

// ResolveWorkspace determines the host directory to mount at /workspace and
// enforces the non-overridable safety refusals: never mount the filesystem root,
// the host home, or an ancestor of the host home. flagPath defaults to cwd when
// empty. The returned path is absolute with symlinks evaluated.
func ResolveWorkspace(flagPath string) (string, error) {
	p := flagPath
	if p == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("determining working directory: %w", err)
		}
		p = wd
	}
	p = config.ExpandTilde(p)

	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolving %q: %w", p, err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("project path does not exist: %q", abs)
	}

	fi, err := os.Stat(real)
	if err != nil || !fi.IsDir() {
		return "", fmt.Errorf("project path is not a directory: %q", real)
	}

	if isFilesystemRoot(real) {
		return "", fmt.Errorf("refusing to mount filesystem root %q as the workspace", real)
	}

	if home := hostHome(); home != "" {
		realHome, herr := filepath.EvalSymlinks(home)
		if herr != nil {
			realHome = home
		}
		switch {
		case real == realHome:
			return "", fmt.Errorf("refusing to mount your home directory %q; cd into a specific project first", real)
		case isAncestor(real, realHome):
			return "", fmt.Errorf("%q is an ancestor of your home directory; too broad to mount safely", real)
		}
	}

	return real, nil
}

// WorkspaceMount builds the /workspace bind mount for the given host path.
func WorkspaceMount(hostPath, target string) runtime.Mount {
	if target == "" {
		target = "/workspace"
	}
	return runtime.Mount{Source: hostPath, Target: target, RO: false}
}

func isFilesystemRoot(p string) bool {
	return p == string(filepath.Separator) || p == filepath.VolumeName(p)+string(filepath.Separator)
}

func hostHome() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// isAncestor reports whether ancestor is a strict parent directory of child.
func isAncestor(ancestor, child string) bool {
	a := filepath.Clean(ancestor)
	c := filepath.Clean(child)
	if a == c {
		return false
	}
	rel, err := filepath.Rel(a, c)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "."
}
