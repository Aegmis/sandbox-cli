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

func TestBuildSpec_SecurityDefaults(t *testing.T) {
	dir := t.TempDir()
	spec, err := BuildSpec(baseCfg(), Options{Project: dir, Command: []string{"sh"}})
	if err != nil {
		t.Fatal(err)
	}
	if !spec.NoNewPrivileges {
		t.Error("expected NoNewPrivileges on by default")
	}
	if len(spec.CapDrop) != 1 || spec.CapDrop[0] != "ALL" {
		t.Errorf("CapDrop = %v, want [ALL]", spec.CapDrop)
	}
	if spec.PidsLimit != 1024 {
		t.Errorf("PidsLimit = %d, want 1024", spec.PidsLimit)
	}
}

func TestBuildSpec_NoHardeningAndResourceFlags(t *testing.T) {
	dir := t.TempDir()
	spec, err := BuildSpec(baseCfg(), Options{
		Project:     dir,
		Command:     []string{"sh"},
		NoHardening: true,
		Memory:      "2g",
		CPUs:        "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.NoNewPrivileges {
		t.Error("--no-hardening should disable NoNewPrivileges")
	}
	if len(spec.CapDrop) != 0 {
		t.Errorf("--no-hardening should clear CapDrop, got %v", spec.CapDrop)
	}
	if spec.PidsLimit != 0 {
		t.Errorf("--no-hardening should clear PidsLimit, got %d", spec.PidsLimit)
	}
	// Resource limits are independent of --no-hardening.
	if spec.Memory != "2g" || spec.CPUs != "1" {
		t.Errorf("resource flags not mapped: mem=%q cpu=%q", spec.Memory, spec.CPUs)
	}
}

func TestBuildSpec_EgressAllowlistFromFlag(t *testing.T) {
	dir := t.TempDir()
	spec, err := BuildSpec(baseCfg(), Options{
		Project: dir,
		Allow:   []string{"internal.example.com"},
		Command: []string{"claude"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Firewall must run as root, drop back to the sandbox user via the entrypoint.
	if spec.User != "root" {
		t.Errorf("User = %q, want root when the firewall is active", spec.User)
	}
	if spec.Entrypoint != "/usr/local/bin/sandbox-firewall" {
		t.Errorf("Entrypoint = %q, want the firewall wrapper", spec.Entrypoint)
	}
	if spec.Env["SANDBOX_RUN_AS"] != "sandbox" {
		t.Errorf("SANDBOX_RUN_AS = %q, want sandbox", spec.Env["SANDBOX_RUN_AS"])
	}
	allow := spec.Env["SANDBOX_EGRESS_ALLOW"]
	if !strings.Contains(allow, "internal.example.com") {
		t.Errorf("SANDBOX_EGRESS_ALLOW missing the flag domain: %q", allow)
	}
	if !strings.Contains(allow, "api.anthropic.com") {
		t.Errorf("SANDBOX_EGRESS_ALLOW missing a baseline domain: %q", allow)
	}
	if !contains(spec.CapAdd, "NET_ADMIN") {
		t.Errorf("CapAdd missing NET_ADMIN: %v", spec.CapAdd)
	}
	// Allowlist implies bridge networking, never "none".
	if spec.Network == "none" {
		t.Error("allowlist must not run with --network none")
	}
}

func TestBuildSpec_AllowlistOverridesNetworkNone(t *testing.T) {
	dir := t.TempDir()
	cfg := baseCfg()
	cfg.Network.Mode = "none"
	spec, err := BuildSpec(cfg, Options{Project: dir, Allow: []string{"x.com"}, Command: []string{"sh"}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Network == "none" {
		t.Error("--allow should switch networking from none to bridge so the allowlist is reachable")
	}
	if spec.Entrypoint == "" {
		t.Error("expected the firewall entrypoint to be set")
	}
}

func TestBuildSpec_NoEgressByDefault(t *testing.T) {
	dir := t.TempDir()
	spec, err := BuildSpec(baseCfg(), Options{Project: dir, Command: []string{"sh"}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Entrypoint != "" {
		t.Errorf("no egress requested but Entrypoint = %q", spec.Entrypoint)
	}
	if _, ok := spec.Env["SANDBOX_EGRESS_ALLOW"]; ok {
		t.Error("no egress requested but SANDBOX_EGRESS_ALLOW is set")
	}
	if spec.User != "sandbox" {
		t.Errorf("User = %q, want sandbox (unchanged) without egress", spec.User)
	}
	if contains(spec.CapAdd, "NET_ADMIN") {
		t.Errorf("unexpected NET_ADMIN without egress: %v", spec.CapAdd)
	}
}

func TestBuildSpec_CacheVolumes(t *testing.T) {
	dir := t.TempDir()

	// Off by default: no volume mounts.
	spec, err := BuildSpec(baseCfg(), Options{Project: dir, Command: []string{"sh"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range spec.Mounts {
		if m.Volume {
			t.Errorf("no cache requested but got a volume mount: %+v", m)
		}
	}

	// --cache adds a named volume per default cache path.
	spec, err = BuildSpec(baseCfg(), Options{Project: dir, Cache: true, Command: []string{"sh"}})
	if err != nil {
		t.Fatal(err)
	}
	var npm *runtime.Mount
	volCount := 0
	for i := range spec.Mounts {
		if spec.Mounts[i].Volume {
			volCount++
			if spec.Mounts[i].Target == "/sandbox/home/.npm" {
				npm = &spec.Mounts[i]
			}
		}
	}
	if volCount == 0 {
		t.Fatal("--cache should add cache volume mounts")
	}
	if npm == nil {
		t.Fatalf("expected an npm cache volume, got mounts %+v", spec.Mounts)
	}
	if npm.Source != "sandbox-cache-npm" {
		t.Errorf("npm cache volume name = %q, want sandbox-cache-npm", npm.Source)
	}
	// The workspace bind mount must still be present and singular.
	binds := 0
	for _, m := range spec.Mounts {
		if !m.Volume {
			binds++
		}
	}
	if binds != 1 {
		t.Errorf("expected exactly one bind mount (workspace), got %d: %+v", binds, spec.Mounts)
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
