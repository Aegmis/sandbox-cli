package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWorkspace_ValidDir(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveWorkspace(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	real, _ := filepath.EvalSymlinks(dir)
	if got != real {
		t.Errorf("got %q, want %q", got, real)
	}
}

func TestResolveWorkspace_RefusesRoot(t *testing.T) {
	if _, err := ResolveWorkspace("/"); err == nil {
		t.Fatal("expected refusal for filesystem root")
	}
}

func TestResolveWorkspace_RefusesHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	if _, err := ResolveWorkspace(home); err == nil {
		t.Fatalf("expected refusal for home directory %q", home)
	}
}

func TestResolveWorkspace_RefusesHomeAncestor(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	parent := filepath.Dir(home)
	if parent == home {
		t.Skip("home has no parent")
	}
	if _, err := ResolveWorkspace(parent); err == nil {
		t.Fatalf("expected refusal for home ancestor %q", parent)
	}
}

func TestResolveWorkspace_NonexistentPath(t *testing.T) {
	if _, err := ResolveWorkspace("/definitely/not/a/real/path/xyz123"); err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestResolveWorkspace_FileNotDir(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "afile")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveWorkspace(f); err == nil {
		t.Fatal("expected error for a file path")
	}
}

func TestIsAncestor(t *testing.T) {
	cases := []struct {
		anc, child string
		want       bool
	}{
		{"/Users", "/Users/amit", true},
		{"/Users/amit", "/Users/amit/proj", true},
		{"/Users/amit", "/Users/amit", false},
		{"/Users/amit/proj", "/Users/amit", false},
		{"/a/b", "/a/bc", false},
	}
	for _, c := range cases {
		if got := isAncestor(c.anc, c.child); got != c.want {
			t.Errorf("isAncestor(%q, %q) = %v, want %v", c.anc, c.child, got, c.want)
		}
	}
}
