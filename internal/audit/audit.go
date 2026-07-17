// Package audit is a seam for recording sandbox sessions. The MVP ships a no-op
// sink; a later milestone can add a JSONL sink capturing image, mounts, command,
// duration, and exit code.
package audit

// SessionMeta describes a single sandbox session for audit purposes.
type SessionMeta struct {
	Image   string
	Workdir string
	Command []string
}

// Sink records session metadata.
type Sink interface {
	RecordSession(meta SessionMeta)
}

// NopSink discards everything. Default in the MVP.
type NopSink struct{}

// RecordSession does nothing.
func (NopSink) RecordSession(SessionMeta) {}
