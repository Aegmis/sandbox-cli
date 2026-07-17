# sandbox-cli — User Guide

Run AI coding agents (Claude Code, Codex CLI) — or any command — inside a
disposable, isolated Docker container. Only your chosen project is visible to the
agent; everything else on your machine stays out of reach. This lets you hand an
agent "Allow All" / `--dangerously-skip-permissions` autonomy while keeping the
blast radius to one project.

- [Why use it](#why-use-it)
- [Requirements](#requirements)
- [Install](#install)
- [Quick start](#quick-start)
- [Everyday use](#everyday-use)
- [Features](#features)
- [Configuration](#configuration)
- [Command reference](#command-reference)
- [Troubleshooting](#troubleshooting)

---

## Why use it

When an agent can run shell commands, a bad `rm -rf`, a prompt-injection attack,
or a stray `cat ~/.aws/credentials` can reach your whole machine. sandbox-cli puts
the agent in a throwaway container where:

- only your **project folder** is mounted (`/workspace`); your home dir, SSH keys,
  and other projects are invisible;
- `HOME` is **fake and ephemeral** — wiped when the container exits (`--rm`);
- **nothing** from your host environment (API keys, tokens) is passed in unless
  you opt in;
- the container is **hardened** (dropped Linux capabilities, no privilege
  escalation, process-count cap) by default.

So you can let the agent go fast without babysitting every action.

---

## Requirements

- **Docker** installed and running (Docker Desktop on macOS/Windows, or a Docker
  daemon on Linux).
- **Go 1.25+** — only to build the CLI (not needed once you have the binary).
- Git — only if you use the `--worktree` feature.

The container image is built automatically on first run — you don't need to pull
or build anything by hand.

---

## Install

```sh
# from a clone of the repo
make build        # -> ./bin/sandbox-cli
# or install onto your PATH
make install      # -> $(go env GOPATH)/bin/sandbox-cli
```

Verify:

```sh
sandbox-cli version
```

The first real run builds the base image (a few minutes, one time).

---

## Quick start

```sh
# 1. Go to a project you want the agent to work on
cd ~/code/my-app

# 2. Run Claude Code in the sandbox (logs you in the first time)
sandbox-cli claude

# ...or Codex
sandbox-cli codex

# 3. Or run any command in the sandbox
sandbox-cli run -- npm test
```

That's it. The agent sees your project at `/workspace` and nothing else.

> **Tip:** add `--dry-run` to any command to print the exact `docker` command it
> would run, without running it. Great for understanding or debugging.

---

## Everyday use

**Let the agent run unattended, safely:**

```sh
sandbox-cli claude --dangerously-skip-permissions
```

Because it's boxed in, "skip permissions" is far less scary — the agent still
can't touch anything outside the project.

**Work on a different project without `cd`:**

```sh
sandbox-cli claude --project ~/code/other-app
```

**Give the agent a helper folder (read-only by default):**

```sh
sandbox-cli run --mount ~/datasets:/workspace/data:ro -- python train.py
```

**Pass an API key in (only this one, only if it's set):**

```sh
sandbox-cli run --env-allow ANTHROPIC_API_KEY -- some-tool
```

**Run several agents at once, each on its own branch:** see
[Parallel agents](#parallel-agents-with-git-worktrees).

---

## Features

### Strong isolation by default
- Only `/workspace` (your project) is writable and connected to the host.
- `HOME`, `/etc`, `/` inside the container are ephemeral and destroyed on exit.
- Runs as a non-root user; your host home is **never** mounted (sandbox-cli
  refuses to mount `/`, your home, or any parent of it).

### Hardened container
Every run drops all Linux capabilities (`--cap-drop=ALL`), forbids privilege
escalation, and caps the process count to blunt fork bombs. Add resource limits
when you want them:

```sh
sandbox-cli run --memory 2g --cpus 1.5 -- npm run build
```

### Network egress allowlist
By default the container has normal network access. To lock it down so the agent
can reach package registries and the model API **but nothing else** (blocking
data exfiltration), use allowlist mode:

```sh
sandbox-cli claude --allow example.com
```

This default-denies outbound traffic and permits only a built-in baseline
(`api.anthropic.com`, `registry.npmjs.org`, `pypi.org`, `github.com`, …) plus any
domains you add — so `npm install` / `pip install` / `git` keep working. *(Needs
a Linux Docker host; resolves domains at startup.)*

### Persistent caches
`--rm` containers normally re-download dependencies every run. Turn on shared,
persistent caches for npm/pip/cargo/go:

```sh
sandbox-cli run --cache -- npm ci
```

### Credential broker
Pass secrets in **without** putting them on the command line, in a config file,
or in your shell history. The value is fetched at run time and forwarded by name:

```sh
# from a file, a command, or a host env var
sandbox-cli claude \
  --secret ANTHROPIC_API_KEY=file:~/.secrets/anthropic \
  --secret GITHUB_TOKEN=cmd:"gh auth token"
```

`cmd:` sources are great for short-lived tokens (`gh auth token`, `op read`,
`vault read`).

### Parallel agents with git worktrees
Run multiple agents at the same time, each on its own branch in its own
container, with no collisions:

```sh
sandbox-cli claude --worktree feature-a -- -p "implement A"
sandbox-cli claude --worktree feature-b -- -p "implement B"   # in parallel

sandbox-cli worktree list        # see them
sandbox-cli worktree rm feature-a
```

The branch is created from your current HEAD if it doesn't exist. Review a result
with a normal `git checkout feature-a`.

### Stronger isolation on demand (microVM / gVisor)
By default you get a normal Docker container (shared kernel). If your host has a
stronger OCI runtime installed, ask for it and get a harder boundary — nothing
else changes:

```sh
sandbox-cli claude --runtime kata-runtime   # microVM: its own kernel
sandbox-cli claude --runtime runsc          # gVisor: userspace-kernel filter
```

*(Requires the runtime to be registered with Docker; Kata needs a Linux host with
nested virtualization.)*

### git & host services that "just work"
```sh
# attribute commits to you + trust the workspace (no "dubious ownership" errors)
sandbox-cli claude --git

# let the agent reach an MCP server running on your host (needed on Linux)
sandbox-cli claude --host-gateway
```

### Live resource metrics
Non-interactive runs show a live memory/CPU gauge; every run prints a peak-usage
summary at the end. `sandbox-cli stats` shows a live table of running sandboxes.
Disable with `--no-metrics`.

### Works with both Claude and Codex
`sandbox-cli claude` and `sandbox-cli codex` wrap each agent, forward its flags
untouched (so `--dangerously-skip-permissions` just works), and **persist each
agent's login** in a sandbox-owned folder so you only log in once — kept separate
from your real `~/.claude`.

---

## Configuration

Zero config is required. To customize, drop a `.sandbox.yaml` in your project
(scaffold one with `sandbox-cli init`):

```yaml
# .sandbox.yaml
workdir: /workspace
user: sandbox                 # non-root by default

mounts:
  - { host: ./data, container: /workspace/data, mode: ro }

env_allow:                    # only these host vars are forwarded, and only if set
  - ANTHROPIC_API_KEY
  - OPENAI_API_KEY

network:
  mode: default               # default | none | allowlist
  allow:                      # extra domains for allowlist mode
    - internal.registry.example.com

security:                     # secure-by-default; override per project
  memory: ""                  # e.g. 2g (opt-in)
  cpus: ""                    # e.g. 1.5 (opt-in)

cache:
  enabled: false              # or use --cache

secrets:
  GITHUB_TOKEN: { command: gh auth token }
  ANTHROPIC_API_KEY: { file: ~/.secrets/anthropic }
```

Settings merge in this order (later wins): built-in defaults →
`~/.config/sandbox/config.yaml` → nearest `.sandbox.yaml` → command-line flags.
Run `sandbox-cli config show` to see the effective, merged config.

---

## Command reference

| Command | What it does |
|---|---|
| `sandbox-cli run -- <cmd>` | Run any command in the sandbox |
| `sandbox-cli claude [args]` | Run Claude Code (args forwarded to the agent) |
| `sandbox-cli codex [args]` | Run Codex CLI |
| `sandbox-cli init` | Scaffold a `.sandbox.yaml` |
| `sandbox-cli config show\|path\|validate` | Inspect the effective config |
| `sandbox-cli stats` | Live table of running sandboxes |
| `sandbox-cli worktree list\|rm` | Manage `--worktree` worktrees |
| `sandbox-cli version` | Print the version |

Common flags (work on `run`/`claude`/`codex`):

| Flag | Meaning |
|---|---|
| `-p, --project DIR` | Host dir to mount at `/workspace` (default: cwd) |
| `-m, --mount H:C[:ro\|rw]` | Extra mount (repeatable) |
| `-e, --env K=V` / `--env-allow NAME` | Set / forward an env var |
| `--allow DOMAIN` | Egress allowlist mode (repeatable) |
| `--cache` | Persist package caches across runs |
| `--secret NAME=file:\|cmd:\|env:...` | Brokered credential (repeatable) |
| `--worktree BRANCH` | Run in a git worktree for BRANCH |
| `--git` | Forward git identity + trust the workspace |
| `--host-gateway` / `--add-host H:IP` | Reach host services / add a host mapping |
| `--memory 2g` / `--cpus 1.5` | Resource limits |
| `--dry-run` | Print the docker command and exit |
| `--build` | Force a rebuild of the base image |

Flag rule for the agent wrappers: sandbox flags come **first**; everything else
is passed straight to the agent. Use `--` to be explicit, e.g.
`sandbox-cli claude --worktree feat -- -p "do the thing"`.

---

## Troubleshooting

**"Cannot connect to the Docker daemon"** — Docker isn't running. Start Docker
Desktop, or your Linux Docker daemon.

**First run is slow** — it's building the base image once. Later runs are fast.
Force a rebuild anytime with `--build`.

**`npm install` fails with `ENOTFOUND` under `--allow`** — the registry isn't in
the allowlist. The common ones are built in; add others with another `--allow`.

**The agent can't reach my local MCP server** — add `--host-gateway` (Linux) and
point the agent at `host.docker.internal`.

**Files in `/workspace` are owned by the wrong user (Linux)** — run as your own
uid: `--user "$(id -u):$(id -g)"`. On macOS Docker Desktop this is handled
automatically.

**I want to see what it will do without running** — add `--dry-run`.

**The agent refuses `--dangerously-skip-permissions` as root** — that's by
design; the default non-root `sandbox` user is what makes skip-permissions safe.
Don't override with `--user root` unless you have a reason.

---

For the security model in depth and how sandbox-cli compares to other tools, see
[README.md](../README.md) and [COMPARISON.md](COMPARISON.md).
