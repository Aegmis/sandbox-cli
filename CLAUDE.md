# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`sandbox-cli` runs AI coding agents (Claude Code, Codex CLI) or any command inside a disposable,
isolated Docker container. Only the chosen project is bind-mounted at `/workspace`; `HOME` is a
fake ephemeral path (`/sandbox/home`) and the container is `--rm`. The goal is to give an agent
"Allow All" autonomy while limiting the blast radius to the project it is editing.

## Commands

```sh
make build              # -> bin/sandbox-cli (embeds version via -ldflags)
make install            # go install ./cmd/sandbox-cli
make test               # unit tests, no Docker required
make test-integration   # end-to-end tests; requires a running Docker daemon (builds base image on first run)
make fmt                # gofmt -w .

# Run a single test
go test ./internal/runtime -run TestBuildArgs
go test -tags docker_integration -run TestClaude ./internal/cli   # a single integration test
```

Integration tests are gated behind the `docker_integration` build tag, so `make test` (plain
`go test ./...`) never touches Docker. Go 1.25+.

## Architecture

Data flows one direction through layered packages, each behind an interface so backends can be
swapped without touching callers:

```
cmd/sandbox-cli  →  internal/cli  →  config.Load + sandbox.BuildSpec  →  runtime.BuildArgs  →  docker
```

- **`internal/config`** — the layered config schema and merge. Precedence (later wins): built-in
  `Default()` → `~/.config/sandbox/config.yaml` → nearest `.sandbox.yaml` (walking up from cwd) →
  CLI flags. Mount host paths are resolved to absolute relative to the file that declared them.
- **`internal/sandbox`** — composition layer. `BuildSpec(cfg, opts)` folds config + per-invocation
  `Options` into a fully-resolved `runtime.RunSpec`. `mounts.go/ResolveWorkspace` enforces the
  **non-overridable safety refusals**: never mount `/`, the host home, or an ancestor of it.
- **`internal/runtime`** — `BuildArgs(RunSpec) []string` is a **pure, deterministic function** that
  produces the `docker` argv. This is the single choke point for the isolation invariants (only
  declared mounts are host-connected; `HOME` is always the fake path; host home is never mounted)
  and is exhaustively unit-tested. `docker_cli.go` is the only backend today, hidden behind the
  `Runtime` interface.
- **`internal/image`** — lazily builds the embedded base image (`assets/Dockerfile`, `//go:embed`)
  on first use via the `Runtime`'s builder hook.
- **`internal/metrics`** — the sticky-footer live resource gauge for non-interactive runs only.
- **`internal/creds`, `internal/audit`** — deliberate **stub seams** for a future credential broker
  and audit trail. Today nothing extra is forwarded and audit goes to a no-op sink; keep these seams clean.

### Two invariants to preserve when changing behavior

1. **Isolation lives in `runtime.BuildArgs` and `sandbox.ResolveWorkspace`.** Any change that could
   affect what the container can reach must keep `internal/runtime/args_test.go` and the `--dry-run`
   golden test (`internal/cli/dryrun_test.go`) honest — update the golden output intentionally, never
   just to make the test pass.

2. **The two subcommand flag-parsing modes are different on purpose** (`internal/cli`):
   - `run` — sandbox flags first, guest command after `--` (`sandbox-cli run --dry-run -- npm test`).
   - `claude` / `codex` wrappers — `DisableFlagParsing: true`; `splitWrapperArgs` consumes a *leading*
     run of recognized sandbox long-flags, then forwards **everything else verbatim** to the agent, so
     `sandbox-cli claude --dangerously-skip-permissions` just works and agent short flags never collide.
     A sandbox option after the boundary needs a `--` separator.

### Agent wrappers

`claude`/`codex` each carry a suggested opt-in env allowlist (e.g. `ANTHROPIC_API_KEY`, applied only
if set) and **persist the agent login by default** by bind-mounting a sandbox-owned host dir
(`~/.config/sandbox/agents/<name>`) at the agent's config dir inside the ephemeral HOME. This is
separate from the host's real `~/.claude`. `--no-persist-auth` opts out.

`claude` additionally read-write mounts the host's Claude history for the current project
(`~/.claude/projects/<bucket>`) into the persisted HOME by default, so host sessions resolve
inside the sandbox and vice versa. `--no-sync` opts out. This is the one default that reaches a
host path outside the workspace — keep it scoped to the single project bucket.

## Conventions

- Non-root by default (`user: sandbox`): agents refuse `--dangerously-skip-permissions` as root, and
  on macOS Docker Desktop bind-mount ownership is virtualized so files are still written as the host user.
- Module path is `github.com/Aegmis/sandbox-cli`. Standard library + `cobra` + `yaml.v3` only.
