# sandbox-cli

Run AI coding agents (Claude Code, Codex CLI) — or any command — inside a
**disposable, isolated Docker container**. Only the project you choose is mounted
at `/workspace`; `HOME` is a fake, ephemeral directory. A mistaken `rm -rf ~` or
a prompt-injected command can't touch the rest of your machine.

```
        Host                                Sandbox (container, --rm)
  ~/projects/myapp  ── bind ──►  /workspace   (the only host-connected path)
  ~/.ssh ~/.aws ~/  ── NOT mounted            HOME=/sandbox/home  (ephemeral)
```

## Why

Developers want to run agents with full autonomy (`--dangerously-skip-permissions`
/ "Allow All") but don't want the agent to have unrestricted access to their host
filesystem and credentials. sandbox-cli gives the agent the convenience of "Allow
All" while limiting the blast radius to the project it's already meant to edit.

## Requirements

- Docker (Docker Desktop on macOS)
- Go 1.25+ to build

## Install

```sh
make install        # go install ./cmd/sandbox-cli
# or
make build          # -> bin/sandbox-cli
```

## Usage

```sh
# Any command in the sandbox
sandbox-cli run -- bash
sandbox-cli run -- sh -c 'echo $HOME; ls /workspace'

# See the exact docker command without running it
sandbox-cli run --dry-run -- npm test

# AI agents (forward their API key from your host env only if it's set)
ANTHROPIC_API_KEY=... sandbox-cli claude
ANTHROPIC_API_KEY=... sandbox-cli claude --dangerously-skip-permissions
OPENAI_API_KEY=...    sandbox-cli codex exec 'run the tests'

# Scaffold a project config
sandbox-cli init
```

### Passing flags to the agent

For `sandbox-cli claude` / `sandbox-cli codex`, **everything you type is forwarded to the
agent** — so `sandbox-cli claude --dangerously-skip-permissions` just works, and there
are no collisions with sandbox's own flags. To set a sandbox option for a wrapped
agent, put it before a `--` separator:

```sh
sandbox-cli claude --project ~/app -- --dangerously-skip-permissions
sandbox-cli codex  --no-tty       -- exec 'run the tests'
```

`sandbox-cli run` uses the opposite default: sandbox flags first, the command after `--`
(`sandbox-cli run --dry-run -- npm test`).

### Persistent agent login

`sandbox-cli claude` / `sandbox-cli codex` **persist the agent's login by default**, so you
authenticate once and it survives the throwaway containers. Each agent's config dir
is bind-mounted from a dedicated, sandbox-owned host directory:

```
~/.config/sandbox/agents/claude  ->  /sandbox/home/.claude   (sandbox-cli claude)
~/.config/sandbox/agents/codex   ->  /sandbox/home/.codex    (sandbox-cli codex)
```

This is **separate from your host `~/.claude`** — the sandbox never reads or writes
your real Claude/Codex config. The first `sandbox-cli claude` prompts you to log in;
subsequent runs reuse the stored credentials. Opt out for a one-off, throwaway
session with `--no-persist-auth`:

```sh
sandbox-cli claude --no-persist-auth
```

The first run builds the `sandbox-base` image (Node + git + common tools, with
Claude Code and Codex pre-installed). Rebuild with `--build`.

### Live resource gauge

For **non-interactive** runs (`--no-tty`, or piped/redirected stdio), sandbox-cli
pins a live resource gauge to the bottom of the terminal showing the container's
memory, CPU, and elapsed time — output scrolls above it, and it's erased when the
run ends:

```
work line 3
 sandbox-cli │ mem 512MiB/7.6GiB ▕▓░░░░░░▏ cpu 82% · 0m47s
```

It is intentionally **not** drawn during an interactive agent session (Claude/Codex
own the full screen). Disable it with `--no-metrics`. Measurement only — no limits
are placed on the container.

### Common flags (run / claude / codex)

| Flag | Meaning |
|---|---|
| `-p, --project` | Host dir mounted at `/workspace` (default: cwd) |
| `-i, --image` | Override the container image |
| `-w, --workdir` | Working dir inside the container |
| `--user` | `sandbox` (non-root default) \| `root` \| `uid:gid` |
| `-m, --mount` | Extra mount `host:container[:ro\|rw]` (repeatable) |
| `-e, --env` | `KEY=VALUE`, or bare `KEY` to forward the host value |
| `--env-allow` | Host env var forwarded only if present (repeatable) |
| `--tty` / `--no-tty` | Force TTY on/off (default: auto-detect) |
| `--dry-run` | Print the docker command and exit |
| `--build` | Force a rebuild of the base image |
| `--no-metrics` | Disable the live resource gauge (non-interactive runs) |

## Configuration

Merged in precedence order (later wins): built-in defaults →
`~/.config/sandbox/config.yaml` → nearest `.sandbox.yaml` (walking up from cwd) →
CLI flags. Run `sandbox-cli config show` to see the effective config.

```yaml
# .sandbox.yaml
image: sandbox-base:0.1.0
workdir: /workspace
user: sandbox           # non-root; agents refuse --dangerously-skip-permissions as root
mounts:
  - { host: ./data, container: /workspace/data, mode: rw }
env:
  NODE_ENV: development
env_allow:            # default-deny: only these host vars are forwarded, if set
  - ANTHROPIC_API_KEY
  - OPENAI_API_KEY
network:
  mode: default       # default | none
```

## Security model (MVP)

- **Only `/workspace` is host-connected and writable.** `HOME`, `/etc`, `/` inside
  the container are ephemeral and destroyed on exit (`--rm`).
- **The host home is never mounted.** sandbox refuses to mount `/`, your home
  directory, or any ancestor of it as the workspace.
- **Default-deny credentials.** Nothing from your host env crosses the boundary
  unless you opt in via `env_allow` / `--env-allow` / `--env`. `claude`/`codex`
  ship a suggested allowlist (e.g. `ANTHROPIC_API_KEY`) applied only if the value
  is set. For OAuth-file logins, mount just the agent's own dir, e.g.
  `--mount ~/.claude:/sandbox/home/.claude:rw`.

Out of scope for this milestone (clean seams exist in the code): credential
broker, network egress allowlists, snapshots, risk scoring, and audit trails.

## Development

```sh
make test              # unit tests (no Docker)
make test-integration  # end-to-end tests (requires Docker)
```

The isolation invariants live in one pure function, `runtime.BuildArgs`, and are
asserted by `internal/runtime/args_test.go` and the `--dry-run` golden test in
`internal/cli/dryrun_test.go`.
