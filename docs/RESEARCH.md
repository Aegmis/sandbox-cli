# sandbox-cli — Landscape Research & Improvement Roadmap

*Snapshot: mid-2026. The agent-sandbox space moves fast; treat specifics as a
point-in-time capture, not a maintained registry.*

## 1. Where sandbox-cli sits today

A single-binary Go **Docker-CLI wrapper**: bind-mounts only the project at `/workspace`,
fake ephemeral `HOME`, `--rm`, non-root, default-deny env forwarding, wraps both `claude`
and `codex`, live resource metrics. Isolation is **container-tier** (shared host kernel).
Default hardening: `--cap-drop=ALL`, `--security-opt no-new-privileges`, `--pids-limit`.

Its edge is **code quality** — one pure/tested `runtime.BuildArgs` choke point for all
docker argv, non-overridable workspace refusals (`sandbox.ResolveWorkspace`), default-deny
credentials, dual-agent support, observability — not a unique isolation capability.

## 2. Competitor landscape — what each provides

Three isolation tiers; sandbox-cli is container-tier (middle): below microVMs, above OS-level.

| Tool | Tier | Agents | Egress control | Standout feature | Weakness |
|---|---|---|---|---|---|
| **Docker `sbx`** (official, GA Jan 2026) | microVM (own kernel + inner Docker daemon) | 7–9 (Claude, Codex, Gemini, Copilot, …) | Proxy: Open / Balanced / Locked-down | **Secrets Manager** injects auth headers so agent never sees raw keys; hardware boundary | OS-limited (Win11/macOS), heavier, closed |
| **Anthropic devcontainer** | container | Claude | **Default-deny iptables+ipset firewall** (`init-firewall.sh`) | Official working **domain egress allowlist** | Reference only, VS Code-centric, self-maintained image |
| **Anthropic `srt`** (Sandbox Runtime) | OS-level (Seatbelt/bubblewrap/WFP) | Claude + MCP + any process | Proxy w/ per-domain prompts | No container; sandboxes MCP + arbitrary processes; embeddable | Shared kernel/FS = softer boundary; beta |
| **Codex built-in** | OS-level (Seatbelt / Landlock+seccomp) | Codex | Network off by default | Zero-infra, built-in, interactive approvals | Codex-only, no reproducible image |
| **claude-pod** (closest twin) | container | Claude | `NET=none` only | Minimal 4-file MIT Docker wrapper | No dual-agent, no tests; **also lacks egress allowlist** |
| **container-use** (Dagger, ~3.9k★) | container per git branch | Claude, Cursor, Goose (MCP) | not detailed | **Parallel agents, one git-worktree/branch each**; auditable history | Dagger dependency, early |
| **Sculptor** (Imbue) | container | Claude+ | not emphasized | GUI, parallel agents, **warm-start cached images** | Same tier, GUI not scriptable |
| **Conductor** (Melty) | git worktree (NOT sandboxed) | Claude, Codex, Cursor | none (runs on host) | Polished multi-agent UX, checkpoints, Linear | Weaker isolation than a container |
| **E2B / Firecracker / microsandbox** | microVM | any (SDK/MCP) | managed/config | Hardware boundary, snapshot-restore, ~100–200ms boot | Cloud/cost or heavier setup; not local single-binary |

**2026 entrants** not in the current `COMPARISON.md`: Vercel Sandbox, Fly.io Sprites,
Daytona ($24M Series A, Feb 2026), Cloudflare Sandboxes, Runloop, Blaxel. Category is
saturated (100+ tools; indexes: wincent gist, restyler/awesome-sandbox, engine.build).

**Key pattern:** two features appear in almost every serious competitor and are **absent
in sandbox-cli** — a **domain egress allowlist** and a **credential broker**. Both are
already scoped as no-op stub seams here (`internal/netpolicy`, `internal/creds`).

## 3. What users actually require (ranked by evidence)

1. **Egress control that's safe AND ergonomic.** #1 need — simultaneously the top
   security ask (exfiltration / prompt-injection; Claude Code itself had a proxy-bypass
   exfil bug across ~130 releases) and the top operational blocker (Codex users hit
   `npm install → ENOTFOUND`). Users call proxy setup "the hardest, least-documented part."
2. **Frictionless YOLO with true host protection** (project-only mount, fake HOME, no host
   creds/SSH reachable). Dominant reason people sandbox. **sandbox-cli already nails this** —
   keep it the headline.
3. **Persistent caches + painless auth across `--rm` runs.** The disposability tax:
   re-downloading node_modules/pip/cargo and re-login each run.
4. **Parallel agents on isolated git worktrees with trivial review.** Fastest-growing workflow.
5. **"Just works" plumbing** — bind-mount UID matching, git/SSH present, MCP
   `host.docker.internal` reachability. The papercuts that make users abandon a wrapper.

## 4. Improvement roadmap — features mapped to needs

| Priority | Feature | Closes | Fit |
|---|---|---|---|
| **P0 ✅ shipped** | `network: allowlist` mode / `--allow DOMAIN` — default-deny egress + baseline domain allowlist (api.anthropic.com, npm, pypi, github…); in-container iptables firewall programmed at startup, then drops to the non-root user | User need #1; biggest competitor gap | Implemented across config/runtime/sandbox/cli + image firewall scripts |
| **P1 ✅ shipped** | Persistent cache volumes — opt-in named Docker volumes for npm/pip/cargo/go/yarn via `cache:` config or `--cache`, shared across runs so `--rm` no longer re-downloads | User need #3 | Volume-mount support in runtime + `CacheSpec` resolution in `BuildSpec` |
| **P1 ✅ shipped (partial)** | Credential broker — `secrets:` / `--secret NAME=file:\|cmd:\|env:` resolves values at run time and forwards them by name, keeping raw secrets off the argv/dry-run/config/shell-history; supports short-lived `cmd:` tokens. Full header-injection (agent never sees the key) still needs a proxy — future work. | Competitor benchmark (`sbx`) | Implemented in `internal/creds` + `BuildSpec`/`Session.Run` |
| **P2 ✅ shipped** | `--worktree BRANCH` runs the sandbox in a git worktree (created if absent, managed under the config dir) so parallel agents each get their own branch/container; `worktree list`/`rm` manage them | User need #4 | New `internal/worktree` package + CLI wiring |
| **P2** | Ergonomics pass — auto UID match, ensure git/SSH, MCP host reachability flag | User need #5 | Small `BuildArgs`/image tweaks |
| **P3 (quick wins)** | Fix stale `init` scaffold (missing `security:` block); refresh `COMPARISON.md`; wire or delete dead `netpolicy`/`creds` seams | Codebase hygiene | Low-risk, immediate |
| Non-goal | MicroVM / hardware boundary | — | Stay container-tier; document honestly |

**Recommendation:** highest-leverage single change is **P0 (egress allowlist)** — the one
feature every research stream independently ranked #1; it fills a seam already designed for
it and turns an honest weakness into a differentiator (safe *and* npm-works-out-of-the-box).

## 5. Sources (selected)

- Docker Sandboxes — [docs](https://docs.docker.com/ai/sandboxes/), [why-microVMs blog](https://www.docker.com/blog/why-microvms-the-architecture-behind-docker-sandboxes/)
- Anthropic — [Claude Code sandboxing](https://www.anthropic.com/engineering/claude-code-sandboxing), devcontainer [`init-firewall.sh`](https://github.com/anthropics/claude-code/blob/main/.devcontainer/init-firewall.sh)
- [anthropic-experimental/sandbox-runtime](https://github.com/anthropic-experimental/sandbox-runtime)
- OpenAI Codex — [agent-approvals-security](https://developers.openai.com/codex/agent-approvals-security), issues [#10612](https://github.com/openai/codex/issues/10612), [#18675](https://github.com/openai/codex/issues/18675)
- [dagger/container-use](https://github.com/dagger/container-use) · [trekhleb/claude-pod](https://github.com/trekhleb/claude-pod) · [rvaidya/claude-code-sandbox](https://github.com/rvaidya/claude-code-sandbox)
- [imbue.com/sculptor](https://imbue.com/sculptor/) · [conductor.build](https://www.conductor.build/) · [microsandbox](https://github.com/microsandbox/microsandbox)
- Claude Code egress bypass writeup — [oddguan.com](https://oddguan.com/blog/second-time-same-sandbox-anthropic-claude-code-network-allowlist-bypass-data-exfiltration/)
- Community indexes — [wincent gist](https://gist.github.com/wincent/2752d8d97727577050c043e4ff9e386e), [restyler/awesome-sandbox](https://github.com/restyler/awesome-sandbox), [engine.build](https://engine.build/lab/agent-sandboxes)
