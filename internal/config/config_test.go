package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	c := Default()
	if c.Workdir != "/workspace" || c.Home != "/sandbox/home" || c.User != "sandbox" {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("default config should validate: %v", err)
	}
}

func TestValidate_BadNetwork(t *testing.T) {
	c := Default()
	c.Network.Mode = "bogus"
	if err := c.Validate(); err == nil {
		t.Error("expected validation error for bad network mode")
	}
}

func TestValidate_BadMountMode(t *testing.T) {
	c := Default()
	c.Mounts = []MountSpec{{Host: "/a", Container: "/b", Mode: "xx"}}
	if err := c.Validate(); err == nil {
		t.Error("expected validation error for bad mount mode")
	}
}

func TestLoad_ProjectOverridesDefault(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, projectFileName)
	content := "image: my-image:9\nuser: sandbox\nnetwork:\n  mode: none\n"
	if err := os.WriteFile(cfgFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// Point user config away so it doesn't interfere.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "empty-xdg"))

	cfg, err := Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Image != "my-image:9" {
		t.Errorf("Image = %q, want my-image:9", cfg.Image)
	}
	if cfg.User != "sandbox" {
		t.Errorf("User = %q, want sandbox", cfg.User)
	}
	if cfg.NetworkArg() != "none" {
		t.Errorf("NetworkArg = %q, want none", cfg.NetworkArg())
	}
	// Unset field falls back to default.
	if cfg.Workdir != "/workspace" {
		t.Errorf("Workdir = %q, want default /workspace", cfg.Workdir)
	}
}

func TestLoad_RelativeMountResolvedAgainstConfigDir(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, projectFileName)
	content := "mounts:\n  - { host: ./data, container: /workspace/data, mode: rw }\n"
	if err := os.WriteFile(cfgFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "empty-xdg"))

	cfg, err := Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(cfg.Mounts))
	}
	want := filepath.Join(dir, "data")
	if cfg.Mounts[0].Host != want {
		t.Errorf("mount host = %q, want %q", cfg.Mounts[0].Host, want)
	}
}

func TestFindProjectConfig_WalksUp(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, projectFileName)
	if err := os.WriteFile(cfgFile, []byte("image: x:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findProjectConfig(sub); got != cfgFile {
		t.Errorf("findProjectConfig = %q, want %q", got, cfgFile)
	}
}
