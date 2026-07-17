// Package sandbox composes config, image building, and the runtime backend into
// a single Session that resolves a request and runs it in an isolated container.
package sandbox

import (
	"context"
	"fmt"

	"github.com/amitghadge/sandbox-cli/internal/audit"
	"github.com/amitghadge/sandbox-cli/internal/config"
	"github.com/amitghadge/sandbox-cli/internal/runtime"
)

// Session ties a resolved config to a runtime backend and an audit sink.
type Session struct {
	Cfg     config.Config
	Runtime runtime.Runtime
	Audit   audit.Sink
}

// New returns a Session with the given config, the docker CLI backend, and a
// no-op audit sink (the audit seam is a stub in the MVP).
func New(cfg config.Config) *Session {
	return &Session{
		Cfg:     cfg,
		Runtime: runtime.NewDockerCLI(),
		Audit:   audit.NopSink{},
	}
}

// Prepare resolves options into a RunSpec without executing anything. Used by
// --dry-run and by Run.
func (s *Session) Prepare(opts Options) (runtime.RunSpec, error) {
	return BuildSpec(s.Cfg, opts)
}

// Run resolves the options and executes the container, returning the guest exit
// code. forceBuild rebuilds the base image even if it already exists locally.
func (s *Session) Run(ctx context.Context, opts Options, forceBuild bool) (int, error) {
	spec, err := s.Prepare(opts)
	if err != nil {
		return 1, err
	}
	if err := s.Runtime.Available(ctx); err != nil {
		return 1, err
	}
	if err := s.Runtime.EnsureImage(ctx, spec.Image, forceBuild); err != nil {
		return 1, fmt.Errorf("preparing image %q: %w", spec.Image, err)
	}

	s.Audit.RecordSession(audit.SessionMeta{
		Image:   spec.Image,
		Workdir: spec.Workdir,
		Command: spec.Command,
	})

	return s.Runtime.Run(ctx, spec)
}
