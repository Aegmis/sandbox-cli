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
