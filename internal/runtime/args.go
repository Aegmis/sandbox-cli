package runtime

import "sort"

// BuildArgs converts a RunSpec into the argument vector for `docker`. It is a
// pure function (no I/O, deterministic) so the isolation invariants can be
// asserted exhaustively in unit tests: only the declared mounts are ever
// host-connected, HOME is always the fake ephemeral path, and the host home is
// never mounted.
func BuildArgs(s RunSpec) []string {
	a := []string{"run", "--init"} // --init reaps zombie children from agent subprocesses

	if s.Remove {
		a = append(a, "--rm")
	}
	if s.TTY {
		a = append(a, "-it")
	} else {
		a = append(a, "-i")
	}
	if s.Hostname != "" {
		a = append(a, "--hostname", s.Hostname)
	}
	if s.User != "" {
		a = append(a, "--user", s.User)
	}
	if s.Network != "" {
		a = append(a, "--network", s.Network)
	}

	for _, m := range s.Mounts {
		mt := "type=bind,source=" + m.Source + ",target=" + m.Target
		if m.RO {
			mt += ",readonly"
		}
		a = append(a, "--mount", mt)
	}

	a = append(a, "-w", s.Workdir)
	if s.Home != "" {
		a = append(a, "-e", "HOME="+s.Home)
	}

	// Explicit key=value pairs, emitted in a stable order for deterministic output.
	for _, k := range sortedKeys(s.Env) {
		a = append(a, "-e", k+"="+s.Env[k])
	}
	// Pass-through by name: docker reads the host value at exec time.
	for _, k := range s.EnvNames {
		a = append(a, "-e", k)
	}

	a = append(a, s.Image)
	a = append(a, s.Command...)
	return a
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
