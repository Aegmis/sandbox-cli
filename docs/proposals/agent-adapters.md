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

### 1. Aider

- [ ] `aider` — the most-installed OSS CLI pair programmer.
- Install: Python (`aider-chat`), not npm. The base image has `python3` but no
  `pip`/`pipx`/`uv` — this adapter needs an image change, and is the reason it is
  worth doing first: it establishes the non-npm bootstrap path the Python agents
  below all reuse.
- Env: `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, `AIDER_MODEL`.
- Config/state: `~/.aider.conf.yml`, `~/.aider/` — under HOME, so persistence works.
- Note: aider drives git directly (auto-commits). Worth testing under `--worktree`,
  where `.git` is a pointer file into the bind-mounted parent repo.

### 2. GitHub Copilot CLI

- [ ] `copilot` — the largest distribution of any agent here.
- Install: npm (`@github/copilot`); verify the current package name.
- Env: `GITHUB_TOKEN`, `GH_TOKEN`, `COPILOT_*`.
- Auth: device-code flow works fine headless — it prints a code you enter on the
  host, no browser needed in the container. State under `~/.config/github-copilot`.
- Note: forwarding a host `GITHUB_TOKEN` hands the container a credential with
  reach far beyond the workspace. Forward-if-set is the existing convention, but
  this one deserves an explicit warning in the command's help text.

### 3. Cursor CLI

- [ ] `cursor-agent`.
- Install: upstream install script (`curl … | bash`), like the claude bootstrap;
  no npm package to bake, so it installs into the persisted HOME on first run.
- Env: `CURSOR_API_KEY`.
- Note: an install-script bootstrap needs network on first run — check it behaves
  under `--allow` (the egress allowlist), whose baseline covers package registries
  but not necessarily the vendor's download host.

### 4. Qwen Code

- [ ] `qwen`.
- Install: npm (`@qwen-code/qwen-code`).
- Env: `OPENAI_API_KEY`/`OPENAI_BASE_URL` (it speaks an OpenAI-compatible API),
  `DASHSCOPE_API_KEY`.
- Note: a Gemini CLI fork, so the `gemini` adapter is the closest template.

### 5. Amp

- [ ] `amp` (Sourcegraph).
- Install: npm (`@sourcegraph/amp`).
- Env: `AMP_API_KEY`, `AMP_URL`.

### 6. Continue CLI

- [ ] `cn`.
- Install: npm (`@continuedev/cli`).
- Env: `CONTINUE_API_KEY` plus provider keys; config `~/.continue`.

### 7. OpenHands CLI

- [ ] `openhands`.
- Install: Python — blocked on the same image work as aider.
- Note: OpenHands normally runs its own runtime container per session. Inside the
  sandbox there is no docker socket (and mounting one would hand the container
  control of the host daemon — see the README's threat model), so this adapter is
  only meaningful for the local/CLI-only runtime mode. Confirm that mode exists
  and works before starting.

### 8. Droid

- [ ] `droid` (Factory).
- Install: upstream install script.
- Env: `FACTORY_API_KEY`.

### 9. Plandex

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
