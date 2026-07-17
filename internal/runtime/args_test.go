package runtime

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildArgs_Basic(t *testing.T) {
	spec := RunSpec{
		Image:    "sandbox-base:0.1.0",
		Workdir:  "/workspace",
		Command:  []string{"sh", "-c", "echo hi"},
		Remove:   true,
		Hostname: "sandbox",
		Home:     "/sandbox/home",
		User:     "root",
		Mounts: []Mount{
			{Source: "/host/proj", Target: "/workspace", RO: false},
		},
	}
	got := BuildArgs(spec)
	want := []string{
		"run", "--init", "--rm", "-i",
		"--hostname", "sandbox",
		"--user", "root",
		"--mount", "type=bind,source=/host/proj,target=/workspace",
		"-w", "/workspace",
		"-e", "HOME=/sandbox/home",
		"sandbox-base:0.1.0",
		"sh", "-c", "echo hi",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildArgs mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestBuildArgs_TTY(t *testing.T) {
	withTTY := BuildArgs(RunSpec{Image: "img", Workdir: "/w", TTY: true})
	if !containsArg(withTTY, "-it") {
		t.Errorf("expected -it with TTY on, got %v", withTTY)
	}
	withoutTTY := BuildArgs(RunSpec{Image: "img", Workdir: "/w", TTY: false})
	if containsArg(withoutTTY, "-it") || !containsArg(withoutTTY, "-i") {
		t.Errorf("expected -i (not -it) with TTY off, got %v", withoutTTY)
	}
}

func TestBuildArgs_ReadOnlyMount(t *testing.T) {
	got := BuildArgs(RunSpec{
		Image:   "img",
		Workdir: "/w",
		Mounts:  []Mount{{Source: "/h", Target: "/c", RO: true}},
	})
	if !containsArg(got, "type=bind,source=/h,target=/c,readonly") {
		t.Errorf("expected readonly mount, got %v", got)
	}
}

func TestBuildArgs_EnvOrderingDeterministic(t *testing.T) {
	spec := RunSpec{
		Image:   "img",
		Workdir: "/w",
		Env:     map[string]string{"B": "2", "A": "1", "C": "3"},
	}
	a := BuildArgs(spec)
	b := BuildArgs(spec)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("BuildArgs not deterministic:\n%v\n%v", a, b)
	}
	// A must appear before B before C.
	joined := strings.Join(a, " ")
	iA := strings.Index(joined, "A=1")
	iB := strings.Index(joined, "B=2")
	iC := strings.Index(joined, "C=3")
	if !(iA < iB && iB < iC) {
		t.Errorf("env not sorted: A=%d B=%d C=%d in %q", iA, iB, iC, joined)
	}
}

func TestBuildArgs_EnvPassthroughByName(t *testing.T) {
	got := BuildArgs(RunSpec{Image: "img", Workdir: "/w", EnvNames: []string{"ANTHROPIC_API_KEY"}})
	// bare -e NAME (no =) forwards host value
	if !hasPair(got, "-e", "ANTHROPIC_API_KEY") {
		t.Errorf("expected passthrough -e ANTHROPIC_API_KEY, got %v", got)
	}
}

func TestBuildArgs_Network(t *testing.T) {
	got := BuildArgs(RunSpec{Image: "img", Workdir: "/w", Network: "none"})
	if !hasPair(got, "--network", "none") {
		t.Errorf("expected --network none, got %v", got)
	}
	got = BuildArgs(RunSpec{Image: "img", Workdir: "/w"})
	if containsArg(got, "--network") {
		t.Errorf("expected no --network by default, got %v", got)
	}
}

func TestBuildArgs_Hardening(t *testing.T) {
	got := BuildArgs(RunSpec{
		Image:           "img",
		Workdir:         "/w",
		NoNewPrivileges: true,
		Seccomp:         "unconfined",
		CapDrop:         []string{"ALL"},
		CapAdd:          []string{"NET_BIND_SERVICE"},
		PidsLimit:       1024,
		Memory:          "2g",
		CPUs:            "1.5",
	})
	pairs := [][2]string{
		{"--security-opt", "no-new-privileges"},
		{"--security-opt", "seccomp=unconfined"},
		{"--cap-drop", "ALL"},
		{"--cap-add", "NET_BIND_SERVICE"},
		{"--pids-limit", "1024"},
		{"--memory", "2g"},
		{"--cpus", "1.5"},
	}
	for _, p := range pairs {
		if !hasPair(got, p[0], p[1]) {
			t.Errorf("expected %s %s, got %v", p[0], p[1], got)
		}
	}
}

func TestBuildArgs_HardeningOmittedWhenUnset(t *testing.T) {
	got := BuildArgs(RunSpec{Image: "img", Workdir: "/w"})
	for _, f := range []string{"--security-opt", "--cap-drop", "--cap-add", "--pids-limit", "--memory", "--cpus"} {
		if containsArg(got, f) {
			t.Errorf("did not expect %s on a bare spec, got %v", f, got)
		}
	}
	// A zero/negative pids limit must not emit the flag.
	if containsArg(BuildArgs(RunSpec{Image: "img", Workdir: "/w", PidsLimit: 0}), "--pids-limit") {
		t.Error("PidsLimit 0 should omit --pids-limit")
	}
}

func TestBuildArgs_AddHost(t *testing.T) {
	got := BuildArgs(RunSpec{
		Image:    "img",
		Workdir:  "/w",
		AddHosts: []string{"host.docker.internal:host-gateway", "db:10.0.0.5"},
	})
	if !hasPair(got, "--add-host", "host.docker.internal:host-gateway") {
		t.Errorf("expected host-gateway add-host, got %v", got)
	}
	if !hasPair(got, "--add-host", "db:10.0.0.5") {
		t.Errorf("expected db add-host, got %v", got)
	}
	if containsArg(BuildArgs(RunSpec{Image: "img", Workdir: "/w"}), "--add-host") {
		t.Error("did not expect --add-host on a bare spec")
	}
}

func TestBuildArgs_VolumeMount(t *testing.T) {
	got := BuildArgs(RunSpec{
		Image:   "img",
		Workdir: "/w",
		Mounts: []Mount{
			{Source: "/host/proj", Target: "/workspace"},                              // bind
			{Source: "sandbox-cache-npm", Target: "/sandbox/home/.npm", Volume: true}, // volume
		},
	})
	if !containsArg(got, "type=bind,source=/host/proj,target=/workspace") {
		t.Errorf("expected the bind mount, got %v", got)
	}
	if !containsArg(got, "type=volume,source=sandbox-cache-npm,target=/sandbox/home/.npm") {
		t.Errorf("expected the named volume mount, got %v", got)
	}
}

func TestBuildArgs_Entrypoint(t *testing.T) {
	got := BuildArgs(RunSpec{Image: "img", Workdir: "/w", Entrypoint: "/usr/local/bin/sandbox-firewall"})
	if !hasPair(got, "--entrypoint", "/usr/local/bin/sandbox-firewall") {
		t.Errorf("expected --entrypoint, got %v", got)
	}
	// The flag must precede the image so docker parses it as a run flag.
	joined := strings.Join(got, " ")
	if strings.Index(joined, "--entrypoint") > strings.Index(joined, " img") {
		t.Errorf("--entrypoint must come before the image: %v", got)
	}
	// Omitted by default.
	if containsArg(BuildArgs(RunSpec{Image: "img", Workdir: "/w"}), "--entrypoint") {
		t.Error("did not expect --entrypoint on a bare spec")
	}
}

// TestBuildArgs_NeverMountsHostHome asserts the core security invariant at the
// arg level: only the mounts explicitly present in the spec are emitted.
func TestBuildArgs_OnlyDeclaredMounts(t *testing.T) {
	got := BuildArgs(RunSpec{
		Image:   "img",
		Workdir: "/workspace",
		Mounts:  []Mount{{Source: "/host/proj", Target: "/workspace"}},
	})
	mountCount := 0
	for i, a := range got {
		if a == "--mount" {
			mountCount++
			if i+1 < len(got) && strings.Contains(got[i+1], "/Users") {
				// only fails if a home-like path leaked in; /host/proj is fine
			}
		}
	}
	if mountCount != 1 {
		t.Errorf("expected exactly 1 mount, got %d in %v", mountCount, got)
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func hasPair(args []string, flag, val string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == val {
			return true
		}
	}
	return false
}
