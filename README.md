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

> New here? Start with the **[User Guide](docs/GUIDE.md)** — setup, everyday
> usage, and a friendly tour of every feature.

## Why

Developers want to run agents with full autonomy (`--dangerously-skip-permissions`
/ "Allow All") but don't want the agent to have unrestricted access to their host
filesystem and credentials. sandbox-cli gives the agent the convenience of "Allow
All" while limiting the blast radius to the project it's already meant to edit.

## Requirements

- Docker (Docker Desktop on macOS)
- Python 3.8+ to run the installer (Go 1.25+ only if you build from source)

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/Aegmis/sandbox-cli/main/install.py | python3 -
```

That detects your OS and CPU, downloads the matching release binary, verifies it
against the release `SHA256SUMS`, and installs it to `~/.local/bin/sandbox-cli`
(`%LOCALAPPDATA%\Programs\sandbox-cli` on Windows) — no root, no package manager.
It prints a PATH hint if that directory isn't on your `PATH`.

<details>
<summary>Other ways to install</summary>

```sh
# a specific release, or a different directory
python3 install.py --version 0.0.1beta.1 --dest ~/bin

# while the repo is private, authenticate with a token
GITHUB_TOKEN=ghp_... curl -fsSL https://raw.githubusercontent.com/Aegmis/sandbox-cli/main/install.py | python3 -

# grab a binary by hand from the releases page
#   https://github.com/Aegmis/sandbox-cli/releases

# build from source (needs Go 1.25+)
make install        # go install ./cmd/sandbox-cli
make build          # -> bin/sandbox-cli
```

Supported release targets: linux, macOS and Windows on amd64 and arm64.
</details>

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
authenticate once and it survives the throwaway containers. A dedicated,
sandbox-owned host directory is bind-mounted as the agent's whole home:

```
~/.config/sandbox/agents/claude  ->  /sandbox/home   (sandbox-cli claude)
~/.config/sandbox/agents/codex   ->  /sandbox/home   (sandbox-cli codex)
```

The whole home is persisted (not just `~/.claude`) because agents keep their
"onboarding done" flag and account info in `~/.claude.json` — a file in the home
root — and write config via atomic rename, which a single-file bind mount can't
capture. This directory is **separate from your host `~/.claude`** — the sandbox
never reads or writes your real Claude/Codex config. The first `sandbox-cli claude`
prompts you to log in; subsequent runs reuse the stored session. Opt out for a
one-off, throwaway session with `--no-persist-auth`:

```sh
sandbox-cli claude --no-persist-auth
```

The first run builds the `sandbox-base` image (Node + git + common tools, with
Claude Code and Codex pre-installed). Rebuild with `--build`.

**Claude Code stays current.** The baked copy is a fallback; on first use
`sandbox-cli claude` installs Claude Code into the persisted HOME (`~/.local`, via
the official installer) where it is writable, so it self-updates from then on and
matches the version you'd get on the host — no rebuild needed.

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
own the full screen). Instead, **every** run (interactive included) prints a one-line
peak-usage summary after it exits — so you still get the numbers for a Claude session:

```
sandbox-cli: peak mem 412MiB · cpu peak 138% · 12m04s
```

The summary is sampled in the background without touching the screen, and is skipped
for containers too short-lived to sample. Disable all of this with `--no-metrics`.
Measurement only — no limits are placed on the container.

**Inside a `sandbox-cli claude` session**, a status line at the bottom of the Claude
UI shows the container's live memory/CPU (via Claude Code's `statusLine`, injected
through a managed-settings file that never touches your own Claude settings):

```
⬢ sandbox · mem 412MiB/7.6GiB · cpu 82%
```

Disable it with `--no-statusline`.

**To watch memory/CPU live for any run**, run `sandbox-cli stats` in a second
terminal — a refreshing table of all running sandbox containers:

```sh
sandbox-cli stats            # live table, refreshes every 2s, Ctrl-C to exit
sandbox-cli stats --once     # a single snapshot (scriptable)
sandbox-cli stats --interval 1s
```

```
sandbox-cli — live stats  15:04:05  (Ctrl-C to exit)

CONTAINER             MEM                CPU     PIDS
sandbox-dk0gtrd15s2g  412MiB / 7.6GiB   82.00%  24
```

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
| `--memory` | Container memory limit, e.g. `2g` (default: unlimited) |
| `--cpus` | Container CPU limit, e.g. `1.5` (default: unlimited) |
| `--no-hardening` | Disable the default cap-drop / no-new-privileges / pids-limit (debug) |
| `--allow` | Enable the egress allowlist and permit a domain, e.g. `--allow example.com` (repeatable; baseline registries always allowed) |
| `--cache` | Persist package-manager caches (npm/pip/cargo/go) in named volumes across runs |
| `--secret` | Brokered credential `NAME=file:PATH \| cmd:COMMAND \| env:VAR`, resolved at run time and kept off the command line (repeatable) |
| `--worktree` | Run in a git worktree for `BRANCH` (created if absent) — parallel per-branch agents |
| `--git` | Forward host git identity and trust the workspace so `git` commits just work in-container |
| `--host-gateway` | Map `host.docker.internal` to the host (reach host MCP servers; needed on Linux) |
| `--add-host` | Extra `HOST:IP` mapping passed to docker (repeatable) |
| `--runtime` | OCI runtime for stronger isolation, e.g. `kata-runtime` (microVM) or `runsc` (gVisor) |

## Parallel agents (git worktrees)

`--worktree BRANCH` runs the sandbox in a dedicated git worktree for `BRANCH`
instead of your working copy, so you can run several agents at once — each on its
own branch, in its own container, with no collisions:

```sh
sandbox-cli claude --worktree feature-a -- -p "implement A"
sandbox-cli claude --worktree feature-b -- -p "implement B"   # in parallel
```

The worktree is created from the current HEAD if the branch doesn't exist, and
lives in a sandbox-owned directory (under `~/.config/sandbox/worktrees/`) so your
project folder stays clean. Review a result with a normal `git checkout BRANCH`.
Manage them with:

```sh
sandbox-cli worktree list        # branch -> path
sandbox-cli worktree rm BRANCH   # remove one when you're done
```

## Stronger isolation (microVM / gVisor)

By default the container is a normal (shared-kernel) Docker container. If your
host has a stronger OCI runtime registered, select it per run for a harder
boundary — no other change to how the sandbox is built:

```sh
sandbox-cli claude --runtime kata-runtime   # microVM: own kernel (hardware boundary)
sandbox-cli claude --runtime runsc          # gVisor: userspace-kernel syscall filter
```

Set it once in config with `runtime: kata-runtime`. This requires the runtime to
be installed and registered with the Docker daemon (e.g. Kata needs a Linux host
with nested virtualization; it is not available on stock macOS Docker Desktop).
Everything else — mounts, hardening, egress allowlist, caches, secrets — works
unchanged on top of it.

## Making git, MCP, and SSH "just work"

- **git** — `--git` forwards your host `user.name` / `user.email` (so commits are
  attributed to you) and marks the mounted workspace as trusted, avoiding git's
  "dubious ownership" refusal when the container user's uid differs from the
  host's. Pairs naturally with `--worktree`.
- **Host MCP servers** — an agent inside the container reaches services on your
  host via `host.docker.internal`. That name resolves automatically on Docker
  Desktop; on Linux add `--host-gateway` (it maps `host.docker.internal` to the
  host gateway). Use `--add-host HOST:IP` for any other host mapping.
- **SSH (manual)** — to push over SSH, forward your agent socket:
  `--mount "$SSH_AUTH_SOCK:/ssh-agent" --env SSH_AUTH_SOCK=/ssh-agent` (on macOS
  Docker Desktop use the socket path `/run/host-services/ssh-auth.sock`).
- **File ownership on Linux** — files written to `/workspace` are owned by the
  container user's uid. On macOS Docker Desktop this is virtualized to your host
  user automatically; on native Linux, run as your own uid with
  `--user "$(id -u):$(id -g)"` if ownership matters (note: the agent's ephemeral
  HOME is owned by the image's `sandbox` user, so prefer this for non-agent runs).

## Platform support

sandbox-cli runs anywhere Docker does. Almost everything works identically across
platforms; the differences are all about the boundary the host can provide.

| Capability | macOS (Docker Desktop) | Linux (native Docker) | Windows (Docker Desktop / WSL2) |
|---|---|---|---|
| Core: `run` / `claude` / `codex`, mounts, env, hardening, metrics | ✅ | ✅ | ✅ |
| `--cache`, `--secret`, `--worktree`, `--git` | ✅ | ✅ | ✅ |
| Egress allowlist (`--allow`) | ✅ ¹ | ✅ | ✅ ¹ |
| `--host-gateway` | auto ² | ✅ (needed) | auto ² |
| `/workspace` file ownership | virtualized to you | container uid ³ | virtualized to you |
| `--runtime kata-runtime` / `runsc` (microVM / gVisor) | ❌ ⁴ | ✅ ⁵ | ❌ ⁴ |

1. The firewall runs `iptables` inside the (Linux) container, so it works wherever
   the container kernel is Linux — including Docker Desktop. Verified in CI on
   native Linux; not yet independently verified on Docker Desktop.
2. `host.docker.internal` resolves automatically on Docker Desktop, so the flag is
   optional there; it's required on native Linux.
3. On native Linux, `/workspace` files are owned by the container user's uid — use
   `--user "$(id -u):$(id -g)"` if that matters. Docker Desktop virtualizes this.
4. Docker Desktop runs containers inside its own managed Linux VM and doesn't allow
   registering custom OCI runtimes — so you can't *select* Kata/gVisor. (You already
   get a VM boundary from Docker Desktop itself.)
5. Requires the runtime registered with the daemon; Kata additionally needs KVM /
   nested virtualization.

**In short:** on macOS/Windows everything works except *selecting* a microVM
runtime — and Docker Desktop already sandboxes containers in a Linux VM. For a
hardware microVM boundary you choose per run, use native Linux with Kata or gVisor.

## Configuration

Merged in precedence order (later wins): built-in defaults →
`~/.config/sandbox/config.yaml` → nearest `.sandbox.yaml` (walking up from cwd) →
CLI flags. Run `sandbox-cli config show` to see the effective config.

```yaml
# .sandbox.yaml
image: sandbox-base:0.1.1
workdir: /workspace
user: sandbox           # non-root; agents refuse --dangerously-skip-permissions as root
# runtime: kata-runtime # stronger isolation (microVM); or runsc for gVisor. default: runc
mounts:
  - { host: ./data, container: /workspace/data, mode: rw }
env:
  NODE_ENV: development
env_allow:            # default-deny: only these host vars are forwarded, if set
  - ANTHROPIC_API_KEY
  - OPENAI_API_KEY
network:
  mode: default       # default | none | allowlist
  allow:              # allowlist mode only: extra domains beyond the baseline
    - internal.registry.example.com
security:             # secure-by-default hardening; override per project/user
  no_new_privileges: true     # block setuid privilege escalation
  cap_drop: [ALL]             # drop all Linux capabilities (cap_add: [] to add back)
  pids_limit: 1024            # fork-bomb guard; 0 disables
  memory: ""                  # e.g. 2g — opt-in, empty = unlimited
  cpus: ""                    # e.g. 1.5 — opt-in, empty = unlimited
cache:                # opt-in: persist package caches across --rm runs
  enabled: false      # or use --cache; mounts named volumes for npm/pip/cargo/go
  paths: []           # extra container cache dirs beyond the defaults
secrets:              # broker: resolve at run time, forward by name (never on the argv/dry-run)
  GITHUB_TOKEN: { command: gh auth token }   # short-lived token from your own tool
  ANTHROPIC_API_KEY: { file: ~/.secrets/anthropic }
  OPENAI_API_KEY: { env: OPENAI_API_KEY }     # from host env, but kept off the command line
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
- **Credential broker.** For secrets you'd rather not put on the command line or
  in a config file, `secrets:` / `--secret NAME=file:PATH|cmd:COMMAND|env:VAR`
  resolves the value at run time (from a file, a host command like `gh auth
  token` / `op read`, or a host env var) and forwards it into the container *by
  name*, so the raw value never appears on the docker argv, in `--dry-run`, in
  config, or in shell history — and `cmd:` sources can be short-lived tokens
  fetched fresh each run. (The agent process still receives the value as an env
  var; hiding it from the agent entirely needs a header-injecting egress proxy,
  which is future work.)
- **Hardened container by default.** Every run drops all Linux capabilities
  (`--cap-drop=ALL`), forbids privilege escalation (`--security-opt
  no-new-privileges`), and caps process count (`--pids-limit`) to blunt fork
  bombs. Tune these under `security:` in config; add memory/CPU limits with
  `--memory` / `--cpus`; or use `--no-hardening` to fall back to the unhardened
  behavior while debugging.
- **Optional egress allowlist.** With `network: allowlist` (or `--allow DOMAIN`),
  outbound traffic is default-denied by an in-container firewall that permits only
  DNS, established flows, a baseline of agent APIs + package registries
  (`api.anthropic.com`, `registry.npmjs.org`, `pypi.org`, `github.com`, …), and any
  domains you add. This lets `npm install` / `pip install` / `git` keep working
  while blocking arbitrary exfiltration from a prompt-injected agent. The firewall
  is programmed at startup (needs `NET_ADMIN`, added only in this mode) and then
  the run drops back to the non-root `sandbox` user; it fails closed if setup
  errors. Requires a Linux-capable Docker host (iptables); resolves domains to IPs
  once at startup, so extremely dynamic CDNs may need extra `allow` entries.

Out of scope for this milestone (clean seams exist in the code): a
header-injecting secrets proxy (so the agent never sees the raw value),
per-request egress policies, snapshots, risk scoring, and audit trails.

## Alternatives & prior art

Running an agent in a disposable container is a crowded space: there are official
options (Docker Sandboxes' `sbx`, Anthropic's devcontainer and Sandbox Runtime,
Codex's built-in OS sandbox) and many open-source twins. sandbox-cli's edge is code
quality and a focused feature set (tested isolation invariants, default-deny env,
dual-agent wrapping, observability) rather than a hard security boundary — for that,
reach for microVM tooling. See [docs/COMPARISON.md](docs/COMPARISON.md) for the full
landscape and an honest comparison.

## Development

```sh
make test              # unit tests (no Docker)
make test-integration  # end-to-end tests (requires Docker)
```

The isolation invariants live in one pure function, `runtime.BuildArgs`, and are
asserted by `internal/runtime/args_test.go` and the `--dry-run` golden test in
`internal/cli/dryrun_test.go`.
