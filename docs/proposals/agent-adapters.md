# Agent adapters: shipped, and the queue

**Status:** four adapters shipped (`claude`, `codex`, `gemini`, `opencode`); the
rest below are a TODO list, ordered most-popular first.
**Code required per adapter:** one file in `internal/cli`, one line in
`root.go`. No image change — see "Why new agents are not baked".

## What an adapter is

An adapter is a thin wrapper subcommand — it adds no isolation of its own and
must not weaken any. Everything that decides what the container can reach still
lives in `runtime.BuildArgs` and `sandbox.ResolveWorkspace`. What the adapter
contributes is the three things that differ per agent:

1. **How the agent is started** — the guest argv. Either the binary name
   (baked into the base image) or a bootstrap script that installs it into the
   persisted HOME on first run (`npmAgentBootstrap` in `internal/cli/agents.go`).
2. **Which host env vars are worth forwarding** — the opt-in `envAllow` list,
   applied only when a variable is actually set on the host. Never forward a
   variable whose value is a *host path* (e.g. `GOOGLE_APPLICATION_CREDENTIALS`):
   the path is not mounted, so the agent fails confusingly instead of prompting
   for auth.
3. **Where its login lives** — a sandbox-owned host dir
   (`~/.config/sandbox/agents/<name>`) mounted as the agent's whole HOME, which
   `finishAgentCmd` wires up. This is separate from the host's real agent config
   by design; an adapter must never mount the host's own `~/.<agent>`.

The shared contract is pinned by `TestAgentWrappersShareTheContract`
(`internal/cli/wrapper_test.go`). A new adapter joins that table.

### Checklist for a new adapter

- [ ] `internal/cli/<agent>.go`: `<agent>EnvAllow` + `new<Agent>Cmd()`, ending in
      `finishAgentCmd(cmd, rf, "<agent>")`.
- [ ] `DisableFlagParsing: true` so agent flags forward verbatim (see the two
      flag-parsing modes in `CLAUDE.md`).
- [ ] Register in `NewRootCmd` (`internal/cli/root.go`).
- [ ] **Do not add it to the Dockerfile.** `agentBootstrap` installs the agent
      into the persisted HOME on first use, so an adapter nobody runs costs the
      image nothing. Baking is reserved for the four agents already there; see
      "Why new agents are not baked" below.
- [ ] Add the agent to the wrapper table in `wrapper_test.go`.
- [ ] Document it: `README.md` (agent list, persisted-login table), `docs/GUIDE.md`.
- [ ] Check whether the agent has a status-line hook (see the table below). If it
      does, wire it; if not, leave the gauge out — see the note there on why the
      tmux workaround was reverted.
- [ ] Confirm the agent's auth and config both land **under HOME**. If an agent
      writes credentials outside HOME, `AuthPersistDir` will not capture them and
      the adapter needs its own mount — call that out in review.

## Shipped

| Agent | Subcommand | Install | Env forwarded (if set) |
|---|---|---|---|
| Claude Code | `claude` | native installer into persisted HOME, npm copy baked as offline fallback | `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_BASE_URL`, `CLAUDE_CODE_USE_BEDROCK`, `CLAUDE_CODE_USE_VERTEX` |
| Codex CLI | `codex` | `@openai/codex`, baked | `OPENAI_API_KEY`, `OPENAI_BASE_URL`, `CODEX_HOME` |
| Gemini CLI | `gemini` | `@google/gemini-cli`, baked + HOME fallback | `GEMINI_API_KEY`, `GOOGLE_API_KEY`, `GOOGLE_GENAI_USE_VERTEXAI`, `GOOGLE_CLOUD_PROJECT`, `GOOGLE_CLOUD_LOCATION` |
| OpenCode | `opencode` | `opencode-ai`, baked + HOME fallback | `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, `GROQ_API_KEY`, `OPENROUTER_API_KEY`, `OPENCODE_CONFIG`, `OPENCODE_DISABLE_AUTOUPDATE` |

| Cline | `cline` | `cline` (npm), installed on first use | `ANTHROPIC_API_KEY`, `CLINE_API_KEY`, `OPENAI_API_KEY`, `OPENROUTER_API_KEY`, `AI_GATEWAY_API_KEY`, `V0_API_KEY` |
| Goose | `goose` | official installer, on first use (needs `bzip2`) | `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GOOGLE_API_KEY`, `GROQ_API_KEY`, `OPENROUTER_API_KEY`, `GOOSE_PROVIDER`, `GOOSE_MODEL`, `GOOSE_FAST_MODEL`, `GOOSE_MODE`; **sets** `GOOSE_DISABLE_KEYRING=1` |
| Crush | `crush` | `@charmland/crush` (npm), installed on first use | `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, `OPENROUTER_API_KEY`, `GROQ_API_KEY`, `HYPER_API_KEY`, AWS/Azure keys |
| Aider | `aider` | `aider-chat` (PyPI) via uv, on first use | `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, `DEEPSEEK_API_KEY`, `OPENROUTER_API_KEY`, `OPENAI_API_BASE`, `ANTHROPIC_API_BASE` |
| GitHub Copilot CLI | `copilot` | `@github/copilot` (npm), installed on first use | `COPILOT_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`, `GH_HOST`, `COPILOT_MODEL`, `COPILOT_API_URL` |
| Cursor CLI | `cursor` | vendor installer, on first use | `CURSOR_API_KEY`, `CURSOR_API_ENDPOINT`; **sets** `NO_OPEN_BROWSER=1` |
| Qwen Code | `qwen` | `@qwen-code/qwen-code` (npm), installed on first use | `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, `GOOGLE_API_KEY`, `DASHSCOPE_API_KEY`, `OPENROUTER_API_KEY`, `BAILIAN_CODING_PLAN_API_KEY`, base URLs; **sets** `SANDBOX=1`, `NO_BROWSER=1` |
| Amp | `amp` | `@ampcode/cli` (npm), installed on first use | `AMP_API_KEY`, `AMP_URL`, `AMP_LOG_LEVEL`, `AMP_SKIP_UPDATE_CHECK` |
| Continue CLI | `continue` (runs `cn`) | `@continuedev/cli` (npm), installed on first use | `ANTHROPIC_API_KEY`, `CONTINUE_API_BASE`, AWS keys, `GOOGLE_CLOUD_PROJECT` |
| OpenHands CLI | `openhands` | standalone binary from GitHub releases, on first use | `LLM_API_KEY`, `LLM_MODEL`, `LLM_BASE_URL` (need `--override-with-envs`), `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `OPENHANDS_CLOUD_URL` |
### Status-line support, per agent

Checked upstream in July 2026, because it is the first thing a new adapter has to
decide and the answer is not guessable:

| Agent | External-command status line? | What sandbox-cli does |
|---|---|---|
| Claude Code | yes — `statusLine` in settings | renders `sandbox-statusline` in the agent's own UI |
| Gemini CLI | no — `ui.footer` toggles built-in items only ([settings schema](https://raw.githubusercontent.com/google-gemini/gemini-cli/main/schemas/settings.schema.json)) | nothing on screen |
| OpenCode | no hook of any kind; [request #30295](https://github.com/anomalyco/opencode/issues/30295) closed unimplemented, and [ocstatusline](https://github.com/amirlehmam/ocstatusline) works around it with a daemon on `opencode serve`'s event stream | nothing on screen |
| Codex CLI | not checked | nothing on screen |

Where there is no hook, there is no gauge; `sandbox-cli stats` in a second terminal
covers it. **Wrapping the agent in tmux to synthesise one was tried and reverted**
(July 2026): a tmux status bar does display the gauge under any TUI, but the agents'
own rendering came out unclear inside it, and a readable agent beats a gauge. If you
revisit this, the thing to establish first is that the agent's UI survives the
multiplexer — the gauge part was never the hard bit. A per-agent native hook remains
the only route worth taking without that evidence.

Aider is the first non-npm adapter. uv carries it: a single static binary that
installs itself and its tools under `~/.local`, needing no root and touching no
system Python. Pin the interpreter with `--python "$(command -v python3)"` or uv
downloads a managed CPython — another ~87MB for one already in the image.
Bookworm ships 3.11 and Aider wants >=3.10,<3.13, so the image's own is fine.
The route the queue assumed (add pip/pipx to the image) turned out to be
unnecessary. Note Aider writes into the *workspace*: a chat history file, a tags
cache, and an appended line in the repo's `.gitignore`.

Goose is the first adapter that had to **set** an env var rather than forward
one. It stores secrets in the OS keyring over DBus; a container has no Secret
Service, so `GOOSE_DISABLE_KEYRING=1` is injected on every run and secrets go to
`~/.config/goose/secrets.yaml` inside the persisted home. Goose does attempt its
own headless fallback, but relying on that would make the login depend on a
heuristic instead of on the run. Its installer also extracts a `.tar.bz2`, which
is why `bzip2` is now in the image — about 100KB, and without it the install
fails. Note the project moved from `block/goose` to `aaif-goose/goose`.

Gemini CLI is also worth knowing for a second reason: it reads a **system**
settings file (`/etc/gemini-cli/settings.json`, overridable with
`GEMINI_CLI_SYSTEM_SETTINGS_PATH`) that outranks user and project settings. That
is the Gemini equivalent of Claude's managed-settings.json — the right place to
put a sandbox-imposed default without touching the user's own config.

Claude alone carries one further extra: the shared host history for the current
project. That is a Claude-specific integration, not part of the adapter contract;
a new adapter should stay in the plain shape unless the agent has a genuine
equivalent.

### Why new agents are not baked

Measured July 2026, unpacked sizes from the npm registry: `@google/gemini-cli`
93 MiB, `@qwen-code/qwen-code` 84 MiB, `@continuedev/cli` 62 MiB. Several others
(`@github/copilot`, `@charmland/crush`) report near-zero because they are stubs
that download a platform binary during install — their real footprint is larger
than the registry number, not smaller. Aider and OpenHands are Python, so baking
them also means adding pip/pipx or uv to the image.

Baking the whole queue would add somewhere between several hundred megabytes and
a gigabyte to a base image every user builds, almost all of it for agents any
given user will never run. So new adapters are installed on first use into
`~/.config/sandbox/agents/<agent>/.local`, which already persists across
containers. An adapter nobody runs costs nothing but its Go file.

The trade, stated honestly: the first launch of each agent waits for a download
and needs network at that moment. `agentBootstrap` announces the install rather
than appearing to hang, and exits 127 with a pointed message when it fails —
including a reminder that under `--allow` the install host has to be on the
allowlist. Vendor install scripts (Cursor, Droid) are the ones most likely to
need a domain adding.

## The queue

Ordered by how widely the agent is used (install base and project traction),
most popular first. That ordering is a judgement call made in **July 2026** and
is the thing most likely to be stale here — re-check before picking one off the
top. Package names and config paths below are the starting point for the work,
**not verified facts**: confirm each against upstream when implementing, since a
wrong package name in the Dockerfile fails silently (`|| true`).

### 1. Droid

- [ ] `droid` (Factory).
- Install: upstream install script.
- Env: `FACTORY_API_KEY`.

### 2. Plandex

- [ ] `plandex` / `pdx`.
- Install: install script or Go binary.
- Note: client/server split — the server can run locally or hosted. The local
  server mode needs a database inside the container; start with hosted mode.

Below this line the agents are niche enough that a user asking for one is a
better signal than this list: Mentat, Codebuff, smol-developer, GPT Engineer,
Kilo/Roo (both primarily IDE extensions with a thin CLI).

## Non-goals

- **One generic `sandbox-cli agent <name>` command** driven by a config table.
  The per-agent files are small, and each one has real differences (bootstrap
  shape, auth flow, provider spread) that a table flattens into guesswork. Adding
  an adapter is a ~50-line file; keep it explicit.
- **Auto-detecting which agent the user has installed on the host.** The
  container's contents are deliberately independent of the host's.
- **Forwarding an agent's host config into the sandbox.** The whole point of the
  sandbox-owned agent home is that a compromised agent cannot reach the real one.
  The Claude history mount is the single exception, scoped to one project bucket.
