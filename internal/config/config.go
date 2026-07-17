// Package config defines the sandbox configuration schema and its layered
// discovery/merge rules: built-in defaults < user config < project config < flags.
package config

import (
	"fmt"

	"github.com/amitghadge/sandbox-cli/internal/version"
)

// Config is the merged sandbox configuration.
type Config struct {
	Image    string            `yaml:"image"`
	Workdir  string            `yaml:"workdir"`
	User     string            `yaml:"user"`
	Home     string            `yaml:"home"`
	Hostname string            `yaml:"hostname"`
	Mounts   []MountSpec       `yaml:"mounts"`
	Env      map[string]string `yaml:"env"`
	EnvAllow []string          `yaml:"env_allow"`
	Network  NetworkSpec       `yaml:"network"`
}

// MountSpec is a bind mount declared in config. Host paths may use ~ and may be
// relative (resolved against the config file's directory when loaded from a file).
type MountSpec struct {
	Host      string `yaml:"host"`
	Container string `yaml:"container"`
	Mode      string `yaml:"mode"` // "ro" | "rw"; empty defaults to "ro"
}

// NetworkSpec controls container networking. MVP honors only Mode.
type NetworkSpec struct {
	Mode string `yaml:"mode"` // "default" | "none"
}

// Default returns the built-in base configuration.
func Default() Config {
	return Config{
		Image:   version.BaseImage(),
		Workdir: "/workspace",
		// Non-root by default: agents like Claude Code refuse
		// --dangerously-skip-permissions when running as root. On macOS Docker
		// Desktop bind-mount ownership is virtualized, so a non-root user still
		// writes /workspace files as the host user. Override with `--user root`.
		User:     "sandbox",
		Home:     "/sandbox/home",
		Hostname: "sandbox",
		Env:      map[string]string{},
		Network:  NetworkSpec{Mode: "default"},
	}
}

// Validate checks that the merged config is internally consistent.
func (c Config) Validate() error {
	if c.Image == "" {
		return fmt.Errorf("image must not be empty")
	}
	if c.Workdir == "" {
		return fmt.Errorf("workdir must not be empty")
	}
	switch c.Network.Mode {
	case "", "default", "none":
	default:
		return fmt.Errorf("network.mode must be \"default\" or \"none\", got %q", c.Network.Mode)
	}
	for i, m := range c.Mounts {
		if m.Host == "" || m.Container == "" {
			return fmt.Errorf("mounts[%d]: host and container are required", i)
		}
		switch m.Mode {
		case "", "ro", "rw":
		default:
			return fmt.Errorf("mounts[%d]: mode must be \"ro\" or \"rw\", got %q", i, m.Mode)
		}
	}
	return nil
}

// NetworkArg maps the config network mode to a docker --network value, or "" for
// the default bridge (no flag emitted).
func (c Config) NetworkArg() string {
	if c.Network.Mode == "none" {
		return "none"
	}
	return ""
}
