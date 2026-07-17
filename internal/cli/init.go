package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const scaffoldConfig = `# sandbox configuration (https://github.com/amitghadge/sandbox-cli)
# Only /workspace (this project) is mounted into the container. HOME is fake and
# ephemeral. Uncomment and edit fields as needed.

# image: sandbox-base:0.1.1
# workdir: /workspace
# user: sandbox         # sandbox (non-root default) | root
#                       # agents refuse --dangerously-skip-permissions as root

# Extra mounts beyond the automatic /workspace bind. Host paths may use ~ and may
# be relative to this file. mode defaults to ro.
# mounts:
#   - { host: ./data, container: /workspace/data, mode: rw }

# Explicit env values injected into the container.
# env:
#   NODE_ENV: development

# Host env vars forwarded ONLY if they are set (default-deny allowlist).
env_allow:
  - ANTHROPIC_API_KEY
  - OPENAI_API_KEY

# Networking: default (bridge) | none | allowlist.
# In allowlist mode, outbound traffic is default-denied except DNS, established
# flows, a baseline of agent APIs + package registries, and the domains below —
# so npm/pip/git keep working while blocking arbitrary exfiltration. (Also
# available ad hoc via --allow DOMAIN.)
network:
  mode: default
  # allow:
  #   - internal.registry.example.com

# Container hardening (secure-by-default; shown here to make it tunable). Pointer
# fields are tri-state: omit to keep the default, set to override.
# security:
#   no_new_privileges: true   # block setuid privilege escalation
#   cap_drop: [ALL]           # drop all Linux capabilities (cap_add: [] to add back)
#   pids_limit: 1024          # fork-bomb guard; 0 disables
#   memory: ""                # e.g. 2g — opt-in, empty = unlimited
#   cpus: ""                  # e.g. 1.5 — opt-in, empty = unlimited
`

func newInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a .sandbox.yaml in the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			path := filepath.Join(wd, ".sandbox.yaml")
			if _, err := os.Stat(path); err == nil && !force {
				return fmt.Errorf("%s already exists (use --force to overwrite)", path)
			}
			if err := os.WriteFile(path, []byte(scaffoldConfig), 0o644); err != nil {
				return err
			}
			fmt.Printf("wrote %s\n", path)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing .sandbox.yaml")
	return cmd
}
