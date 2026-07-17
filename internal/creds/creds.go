// Package creds is a seam for credential brokering. The MVP forwards nothing by
// default and only honors an opt-in env allowlist (applied in sandbox.BuildSpec).
// A later milestone can add a keychain/vault-backed broker issuing short-lived,
// scoped tokens so the agent never sees raw long-lived credentials.
package creds

import "github.com/amitghadge/sandbox-cli/internal/runtime"

// EnvVar is a resolved environment variable to inject.
type EnvVar struct {
	Name  string
	Value string
}

// Broker resolves the credentials a given agent is allowed to receive, as env
// vars and/or scoped mounts.
type Broker interface {
	Resolve(agent string) (env []EnvVar, mounts []runtime.Mount, err error)
}

// AllowlistBroker is the MVP implementation: it grants nothing beyond what the
// env allowlist already handles. It exists to make the seam concrete.
type AllowlistBroker struct{}

// Resolve returns no additional credentials in the MVP.
func (AllowlistBroker) Resolve(string) ([]EnvVar, []runtime.Mount, error) {
	return nil, nil, nil
}
