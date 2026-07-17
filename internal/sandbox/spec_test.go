package sandbox

import (
	"strings"
	"testing"

	"github.com/amitghadge/sandbox-cli/internal/config"
	"github.com/amitghadge/sandbox-cli/internal/runtime"
)

func baseCfg() config.Config {
	c := config.Default()
	return c
}

func TestBuildSpec_WorkspaceMountAndHome(t *testing.T) {
	dir := t.TempDir()
	spec, err := BuildSpec(baseCfg(), Options{Project: dir, Command: []string{"sh"}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Home != "/sandbox/home" {
		t.Errorf("Home = %q, want /sandbox/home", spec.Home)
	}
	if len(spec.Mounts) != 1 || spec.Mounts[0].Target != "/workspace" {
		t.Fatalf("expected single /workspace mount, got %+v", spec.Mounts)
	}
	// No mount may point at a HOME-like location; only the temp project dir.
	for _, m := range spec.Mounts {
		if strings.HasSuffix(m.Target, "home") {
			t.Errorf("unexpected home mount: %+v", m)
		}
	}
}

func TestBuildSpec_ExplicitEnv(t *testing.T) {
	dir := t.TempDir()
	spec, err := BuildSpec(baseCfg(), Options{Project: dir, Env: []string{"FOO=bar"}, Command: []string{"sh"}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Env["FOO"] != "bar" {
		t.Errorf("Env[FOO] = %q, want bar", spec.Env["FOO"])
	}
}

func TestBuildSpec_EnvAllowOnlyIfPresent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PRESENT_KEY", "v")
	spec, err := BuildSpec(baseCfg(), Options{
		Project:  dir,
		EnvAllow: []string{"PRESENT_KEY", "ABSENT_KEY_XYZ"},
		Command:  []string{"sh"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(spec.EnvNames, "PRESENT_KEY") {
		t.Errorf("expected PRESENT_KEY forwarded, got %v", spec.EnvNames)
	}
	if contains(spec.EnvNames, "ABSENT_KEY_XYZ") {
		t.Errorf("did not expect ABSENT_KEY_XYZ, got %v", spec.EnvNames)
	}
}

func TestBuildSpec_FlagMounts(t *testing.T) {
	dir := t.TempDir()
	spec, err := BuildSpec(baseCfg(), Options{
		Project:     dir,
		ExtraMounts: []string{"/h/data:/workspace/data:rw", "/h/cfg:/etc/cfg"},
		Command:     []string{"sh"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var rw, ro *runtime.Mount
	for i := range spec.Mounts {
		switch spec.Mounts[i].Target {
		case "/workspace/data":
			rw = &spec.Mounts[i]
		case "/etc/cfg":
			ro = &spec.Mounts[i]
		}
	}
	if rw == nil || rw.RO {
		t.Errorf("expected /workspace/data as rw, got %+v", rw)
	}
	if ro == nil || !ro.RO {
		t.Errorf("expected /etc/cfg as ro (default), got %+v", ro)
	}
}

func TestBuildSpec_BadMount(t *testing.T) {
	dir := t.TempDir()
	_, err := BuildSpec(baseCfg(), Options{Project: dir, ExtraMounts: []string{"onlyone"}, Command: []string{"sh"}})
	if err == nil {
		t.Fatal("expected error for malformed mount")
	}
}

func TestBuildSpec_ImageAndUserOverride(t *testing.T) {
	dir := t.TempDir()
	spec, err := BuildSpec(baseCfg(), Options{Project: dir, Image: "custom:1", User: "sandbox", Command: []string{"sh"}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Image != "custom:1" {
		t.Errorf("Image = %q, want custom:1", spec.Image)
	}
	if spec.User != "sandbox" {
		t.Errorf("User = %q, want sandbox", spec.User)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
