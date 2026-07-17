# Alternatives & prior art

*Landscape as of mid-2026. This space moves fast — treat the specifics below as a
snapshot, not a maintained registry.*

Running an AI coding agent (Claude Code, Codex CLI) inside a disposable, isolated
container is **not a novel idea** — it's one of the most crowded corners of the
agent-tooling ecosystem. This document places `sandbox-cli` honestly among the
alternatives so you can decide whether it fits your needs, and so the project's
deliberate non-goals read as choices rather than gaps.

## Where sandbox-cli fits

`sandbox-cli` is a **Docker-CLI wrapper**: it bind-mounts only your chosen project
at `/workspace`, gives the container a fake ephemeral `HOME`, runs `--rm` as a
non-root user, and forwards nothing from your host environment unless you opt in.
Its isolation is **container-tier** — a Linux container boundary — not a
hardware-virtualized microVM and not an OS-level (Seatbelt/Landlock) sandbox. It
wraps **both** `claude` and `codex`, persists each agent's login separately from
your real host config, and adds live resource observability.

That places it in the same category as a cluster of open-source "twins," below the
official microVM tooling on the isolation-strength axis, and above the raw OS-level
sandboxes on the convenience/reproducibility axis.

## The landscape

### First-party / official

| Tool | Isolation | Agents |
|---|---|---|
| **Docker Sandboxes (`sbx` CLI)** — GA Jan 2026; per-agent microVM with its own kernel and inner Docker daemon. Host-side proxy with **Open / Balanced / Locked-down** egress modes (blocks raw TCP/UDP, private IPs, loopback), plus a **Secrets Manager** that injects auth headers so the agent never sees raw API keys. Free, incl. commercial use; **Windows 11 x86_64 / macOS arm64 only** (needs HypervisorPlatform). | microVM | Claude, **Codex**, Gemini, Copilot, OpenCode, Kiro, Docker Agent (7–9) |
| **Anthropic devcontainer** — official `anthropics/claude-code` example: CLI + `init-firewall.sh` (default-deny iptables/ipset egress allowlist) + persistent volumes. A reference, not a maintained base image. | container (devcontainer) | Claude |
| **Anthropic Sandbox Runtime (`srt`)** — official OS-level sandbox: Seatbelt (macOS) / bubblewrap (Linux) / **WFP filter (Windows)** + network-filtering proxy with per-domain prompts; backs Claude Code's sandboxed Bash tool and can also sandbox local MCP servers and arbitrary processes. ~3–4k★, ~171k npm downloads. | OS-level | Claude (+ MCP, any process) |
| **Codex built-in sandbox** — default OS-level: Seatbelt on macOS, Landlock + seccomp on Linux; network off by default, writes confined to the workspace, plus an approval layer. | OS-level | Codex |
| **Claude Code built-in sandboxed Bash** — restricts Bash commands only; file tools, MCP servers, and hooks still run on the host. | OS-level (partial) | Claude |

### Direct twins — Docker wrapper, project-only mount, `--rm`

These implement essentially the same pattern as `sandbox-cli`:

- **`claude-pod`** (trekhleb) — the closest twin: mounts one project folder,
  `--rm` ephemeral, `HOME`/`~/.ssh` invisible via mount namespaces, login persisted
  in `~/.claude-pod/`, with `NET=none` and CPU/mem/PID limits. Shell + Dockerfile,
  Claude-only.
- **`rvaidya/claude-code-sandbox`** — cwd → `/workspace`, per-project images,
  auto-enables `--dangerously-skip-permissions` inside the container.
- **`Z7Lab/claude-code-sandbox`** — `--rm` ephemeral, auth cached in a mounted dir.
- **`trailofbits/claude-code-devcontainer`** — hardened devcontainer built for
  untrusted-code review; blast radius confined to `/workspace`.
- **`ClaudeBox`, `sandclaude`** — opinionated Docker wrappers for Claude with
  command allowlists.
- **Codex-side:** `codex-lockbox` (Docker + firewall rules), `codex-container-sandbox`
  (Podman), `packnplay`, `agentbox`.
- **`textcortex/claude-code-sandbox`** — an early, popular entry, now **archived**
  (Feb 2026; continued as "Spritz").

### Adjacent / multi-agent orchestration

`container-use` (Dagger — containerized git-worktrees via MCP; ~3.9k★, Apache-2.0),
`Sculptor` (Imbue — GUI, parallel containers, warm-start cached images), `Conductor`
(Melty Labs), `cco`, `yoloAI` (multi-backend over Seatbelt/Tart/Docker). Note:
**Conductor isolates via git worktrees, not containers/VMs** — its agents run on the
host, so it is not a sandbox in the security sense (its container-tier peers are stronger
on the safety axis).

### Stronger isolation tiers

- **microVM:** Firecracker, E2B, Fly.io Sprites, Vercel Sandbox, Daytona ($24M Series A,
  Feb 2026), Cloudflare Sandboxes, Modal, Runloop, Northflank (Kata), Blaxel, Kata
  Containers, `microsandbox`.
- **OS-level:** bubblewrap, Landlock, Firejail, nsjail, gVisor.

Community indexes — the wincent gist ("List of coding-agent sandboxes," May 2026),
`restyler/awesome-sandbox`, and engine.build's "26 providers compared" — catalog 100+
tools across these tiers; the category is genuinely saturated.

## How sandbox-cli compares

### Where it stands out

- **A single, tested isolation choke point.** All docker argv is produced by one
  pure function, `runtime.BuildArgs`, and the non-overridable workspace refusals
  (never mount `/`, the host home, or an ancestor of it) live in
  `sandbox.ResolveWorkspace`. Both are exhaustively unit-tested, with a golden
  `--dry-run` test asserting the invariants. That test discipline exceeds most of
  the shell-script twins.
- **Default-deny credential forwarding.** Nothing from your host env crosses the
  boundary unless allowlisted, and forwarded values are never printed in the
  dry-run command — vs. tools that blanket-discover and forward host credentials.
- **Per-agent persistent login, isolated from your real config.** Each agent's home
  is persisted in a sandbox-owned dir, kept separate from your host `~/.claude`,
  surviving `--rm` containers.
- **Wraps both `claude` and `codex`** with a leading-flag split so
  `--dangerously-skip-permissions` "just works" without colliding with sandbox flags.
- **Built-in resource observability** — live gauge, post-run peak summary, `stats`
  table, and an in-Claude-UI status line.
- **Hardened by default** — every run drops all capabilities (`--cap-drop=ALL`),
  sets `no-new-privileges`, and caps process count (`--pids-limit`); memory/CPU
  limits are one flag away (`--memory` / `--cpus`). On par with the twins that
  ship resource caps, and configurable per project via a `security:` block.
- **Optional egress allowlist** — `network: allowlist` / `--allow DOMAIN` default-denies
  outbound traffic with an in-container firewall, permitting only a baseline of agent
  APIs + package registries plus your domains, so `npm`/`pip`/`git` keep working while
  blocking exfiltration. Closes the gap that even the closest twin (`claude-pod`)
  explicitly leaves open; comparable in spirit to the devcontainer firewall.
- **Credential broker** — `secrets:` / `--secret NAME=file:|cmd:|env:` resolves secrets at
  run time (a file, a host command like `gh auth token` / `op read`, or a host env var)
  and forwards them by name, keeping the raw value off the command line, out of
  `--dry-run`, out of config, and out of shell history; `cmd:` sources allow short-lived
  tokens. Most twins just pass `-e KEY=VALUE` (secret on the argv).
- **Parallel per-branch agents** — `--worktree BRANCH` runs the sandbox in a git worktree
  (created if needed, managed under the config dir), so several agents run at once, each on
  its own branch in its own container, reviewed with a plain `git checkout`. This is the
  workflow `container-use` and Conductor are built around, here as a one flag on the same
  Docker wrapper.

### Deliberate non-goals / current gaps

- **Container-tier, not microVM.** A Linux container is a softer boundary than the
  hardware-virtualized microVMs used by Docker `sbx`, E2B, or Firecracker (and
  softer than Docker Desktop 4.58+'s microVM layer). If you need a hard hypervisor
  boundary, reach for those.
- **Egress allowlist is IP-based, not a full proxy.** `network: allowlist` / `--allow`
  now default-denies egress and permits a baseline + your domains, but it resolves
  domains to IPs once at startup (very dynamic CDNs may need extra entries) and is not
  yet an SNI/domain-level egress proxy like `sbx`/`srt`. Enforcement is container-tier
  (iptables + `NET_ADMIN`), so it needs a Linux-capable Docker host.
- **No user-namespace remapping or `--read-only` rootfs yet** — the next hardening
  steps after the baseline below.
- **Credential broker resolves + forwards, but doesn't hide from the agent.**
  `secrets:` / `--secret` now resolve values at run time (file / host command /
  env) and forward them by name, keeping raw secrets off the argv, `--dry-run`,
  config, and shell history — but the agent process still receives the value. A
  header-injecting proxy so the agent never sees the key (like `sbx`'s Secrets
  Manager) is the remaining gap. The audit sink (`internal/audit`) is still a
  deliberate no-op seam.

## Bottom line

`sandbox-cli` is a clean, well-tested implementation of a well-trodden pattern. Its
edge is **code quality and a focused feature set** (tested isolation invariants,
default-deny env, dual-agent wrapping, observability) rather than a unique
capability. If your priority is a hard security boundary, the official microVM
tooling (`sbx`) or a Firecracker-based SaaS is stronger; if you want the lightest
local OS-level sandbox, `srt` / Codex's built-in mode is thinner. `sandbox-cli`
aims at the middle: a reproducible, scriptable, single-binary container wrapper with
honest isolation invariants and room to harden (see the seams above).

## Sources

- [Docker Sandboxes — product page](https://www.docker.com/products/docker-sandboxes/)
- [Docker Sandboxes — docs](https://docs.docker.com/ai/sandboxes/)
- [Docker Sandboxes — Claude Code](https://docs.docker.com/ai/sandboxes/agents/claude-code/)
- [Docker blog — Run Claude Code and other coding agents safely](https://www.docker.com/blog/docker-sandboxes-run-claude-code-and-other-coding-agents-unsupervised-but-safely/)
- [Claude Code docs — Choose a sandbox environment](https://code.claude.com/docs/en/sandbox-environments)
- [Claude Code docs — Development containers](https://code.claude.com/docs/en/devcontainer)
- [Codex — Agent approvals & security](https://developers.openai.com/codex/agent-approvals-security)
- [Codex Knowledge Base — Docker Sandboxes / microVM isolation](https://codex.danielvaughan.com/2026/04/13/docker-sandboxes-codex-cli-microvm-isolation/)
- [Codex Knowledge Base — permission profiles & sandbox modes](https://codex.danielvaughan.com/2026/05/08/codex-cli-permission-profiles-sandbox-modes-security-layers/)
- [claude-pod (trekhleb)](https://github.com/trekhleb/claude-pod)
- [claude-code-sandbox (rvaidya)](https://github.com/rvaidya/claude-code-sandbox)
- [claude-code-sandbox (Z7Lab)](https://github.com/Z7Lab/claude-code-sandbox)
- [claude-code-devcontainer (trailofbits)](https://github.com/trailofbits/claude-code-devcontainer)
- [claude-code-sandbox (textcortex, archived)](https://github.com/textcortex/claude-code-sandbox)
- [List of coding-agent sandboxes (community index, 2026-05)](https://gist.github.com/wincent/2752d8d97727577050c043e4ff9e386e)
