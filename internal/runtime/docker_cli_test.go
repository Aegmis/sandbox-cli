package runtime

import (
	"strings"
	"testing"
)

func TestParseRuntimeNames(t *testing.T) {
	// Shape of `docker info --format '{{json .Runtimes}}'`.
	out := []byte(`{"io.containerd.runc.v2":{"path":"runc"},"runc":{"path":"runc"},"runsc":{"path":"/usr/local/bin/runsc"}}`)
	got, err := parseRuntimeNames(out)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"io.containerd.runc.v2", "runc", "runsc"} // sorted
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("parseRuntimeNames = %v, want %v", got, want)
	}
	if _, err := parseRuntimeNames([]byte("not json")); err == nil {
		t.Error("expected an error on malformed JSON")
	}
}

func TestRuntimeHint(t *testing.T) {
	// Registered runtime -> no error.
	if err := runtimeHint("runsc", []string{"runc", "runsc"}); err != nil {
		t.Errorf("registered runtime should pass, got %v", err)
	}
	// Unregistered -> actionable error naming the runtime and what's available.
	err := runtimeHint("kata-runtime", []string{"runc", "runsc"})
	if err == nil {
		t.Fatal("expected an error for an unregistered runtime")
	}
	msg := err.Error()
	for _, want := range []string{"kata-runtime", "not registered", "runc, runsc", "runsc install"} {
		if !strings.Contains(msg, want) {
			t.Errorf("hint missing %q; got: %s", want, msg)
		}
	}
	// Empty availability still yields a sensible message.
	if !strings.Contains(runtimeHint("x", nil).Error(), "(none reported)") {
		t.Error("expected a placeholder when no runtimes are reported")
	}
}
