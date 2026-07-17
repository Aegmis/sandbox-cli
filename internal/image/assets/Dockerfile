# sandbox-base: the default disposable execution environment.
#
# node:*-bookworm-slim is multi-arch, so on Apple Silicon this resolves to the
# arm64 variant automatically. Node is present because Claude Code and Codex are
# distributed as npm packages.
FROM node:22-bookworm-slim

# Common dev/agent tooling.
RUN apt-get update && apt-get install -y --no-install-recommends \
      git \
      curl \
      ca-certificates \
      ripgrep \
      less \
      openssh-client \
      bash \
      jq \
      python3 \
      build-essential \
    && rm -rf /var/lib/apt/lists/*

# Pre-install the agents so `sandbox claude` / `sandbox codex` work out of the box.
# Kept best-effort so an upstream hiccup doesn't break the whole image build.
RUN npm install -g @anthropic-ai/claude-code @openai/codex || true

# Isolated, non-root user whose home IS /sandbox/home — set as the passwd home
# (-d) so $HOME and getpwuid agree and there is no stray /home/sandbox. Nothing
# here is host-connected: the only host mount is /workspace, injected at run
# time. No explicit UID: the base node image already uses UID 1000, so we let
# useradd pick a free one.
RUN useradd -m -d /sandbox/home -s /bin/bash sandbox \
    && mkdir -p /workspace \
    && chown -R sandbox:sandbox /sandbox/home /workspace

ENV HOME=/sandbox/home
WORKDIR /workspace

# MVP default is root (safe on macOS Docker Desktop where bind ownership is
# virtualized); override at run time with `--user sandbox`.
