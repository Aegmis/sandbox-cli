package cli

import (
	"os"
	"path/filepath"
)

// sharedTarget is where --share mounts the shared host directory inside the
// container. Deliberately short and well-known: the whole point is that it can
// be named in a sentence to an agent ("write the contract to
// /shared/openapi.yaml") without anyone learning a config schema first.
const sharedTarget = "/shared"

// sharedReadme is seeded into the shared directory the first time --share
// creates it. A bind mount is invisible to an agent that was never told it
// exists; a README inside the directory it is about to list is the cheapest way
// to make the channel self-describing to both the agent and the user who later
// wonders what this folder is.
const sharedReadme = `# Shared sandbox directory

This directory is mounted at ` + "`" + sharedTarget + "`" + ` inside every sandbox started with
` + "`--share`" + `, and lives on the host at ` + "`~/.config/sandbox/shared`" + `.

It is the one place two sandboxes can exchange files. Everything else a sandbox
can see is scoped to its own project, so agents working in different projects
(or different git worktrees) have no other way to hand something over.

Use it for artifacts that cross a boundary — an API contract, a JSON schema, a
generated client, a note from one agent to another:

    # in the sandbox that produces it
    write the API contract to /shared/openapi.yaml

    # in the sandbox that consumes it
    read /shared/openapi.yaml and implement the endpoints

Files written here persist on the host after the containers exit, and are shared
read-write by every sandbox using --share, so treat it as scratch space with one
owner per file rather than a database. For versioned handover, keep a git repo
in here and push to it from both sides.
`

// seedSharedReadme writes the explainer into dir unless something is already
// there. Best-effort by design: the mount is the part that matters and has
// already been arranged by the caller, so a failure to write a README must never
// fail the run.
func seedSharedReadme(dir string) {
	p := filepath.Join(dir, "README.md")
	if _, err := os.Stat(p); err == nil {
		return
	}
	_ = os.WriteFile(p, []byte(sharedReadme), 0o600)
}
