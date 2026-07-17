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
      iptables \
    && rm -rf /var/lib/apt/lists/*

# Pre-install the agents so `sandbox-cli codex` and an offline `sandbox-cli claude`
# work out of the box. Kept best-effort so an upstream hiccup doesn't break the
# build. The claude wrapper prefers a self-updating install in the persisted HOME
# (~/.local/bin, ahead on PATH); this baked copy is the offline fallback.
RUN npm install -g @anthropic-ai/claude-code @openai/codex || true

# Live resource status line for Claude Code: reads the container's own cgroup v2
# memory/CPU and prints one line. Claude runs it via a managed-settings.json that
# sandbox-cli mounts at /etc/claude-code/managed-settings.json at run time.
RUN cat > /usr/local/bin/sandbox-statusline <<'SL'
#!/bin/sh
# Claude pipes the session JSON on stdin; consume it so the pipe closes cleanly.
cat >/dev/null 2>&1 || true

cg=/sys/fs/cgroup
# Current working-set memory, computed the same way `docker stats` does:
# memory.current minus reclaimable page cache (inactive_file). Usage only — no
# total (the machine total was both misleading and hit a mawk 32-bit overflow).
cur=$(cat "$cg/memory.current" 2>/dev/null || echo 0)
inact=$(awk '/^inactive_file /{print $2; exit}' "$cg/memory.stat" 2>/dev/null || echo 0)
used=$(( cur - inact ))
[ "$used" -ge 0 ] 2>/dev/null || used=$cur
usage=$(awk '/^usage_usec/{print $2; exit}' "$cg/cpu.stat" 2>/dev/null || echo 0)
now=$(date +%s%N 2>/dev/null || echo 0)

state=/tmp/.sandbox-statusline
cpu=""
if [ -r "$state" ]; then
  read -r pu pn < "$state" 2>/dev/null || true
  if [ -n "${pn:-}" ] && [ "$now" -gt "$pn" ] 2>/dev/null; then
    du=$(( usage - pu ))
    dw=$(( (now - pn) / 1000 ))
    if [ "$dw" -gt 0 ] && [ "$du" -ge 0 ]; then
      cpu=$(awk "BEGIN{printf \"%.0f\", ($du/$dw)*100}")
    fi
  fi
fi
printf '%s %s\n' "$usage" "$now" > "$state" 2>/dev/null || true

hum() { awk -v b="${1:-0}" 'BEGIN{split("B KiB MiB GiB TiB",u," ");i=1;while(b>=1024&&i<5){b/=1024;i++}printf (i==1?"%.0f%s":"%.1f%s"),b,u[i]}'; }

line="⬢ sandbox · mem $(hum "$used")"
if [ -n "$cpu" ]; then line="$line · cpu ${cpu}%"; fi
printf '\033[2m%s\033[0m\n' "$line"
SL
RUN chmod +x /usr/local/bin/sandbox-statusline

# Egress allowlist support (used only when sandbox-cli runs with `network:
# allowlist` / `--allow`). sandbox-egress-setup programs a default-deny OUTPUT
# firewall that permits loopback, DNS, established flows, and the resolved IPs of
# the allowlisted domains (passed as SANDBOX_EGRESS_ALLOW). It needs to run as
# root with NET_ADMIN, before the agent starts. Uses plain iptables (no ipset) so
# it works on the common iptables-nft backend without extra kernel modules.
RUN cat > /usr/local/bin/sandbox-egress-setup <<'FW'
#!/usr/bin/env bash
set -euo pipefail

allow="${SANDBOX_EGRESS_ALLOW:-}"

# No IPv6 allowlist yet: reject all IPv6 egress so it can't be used to bypass the
# IPv4 rules. Best-effort — some hosts have no ip6tables.
if command -v ip6tables >/dev/null 2>&1; then
  ip6tables -F OUTPUT 2>/dev/null || true
  ip6tables -A OUTPUT -o lo -j ACCEPT 2>/dev/null || true
  ip6tables -A OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || true
  ip6tables -A OUTPUT -j REJECT 2>/dev/null || true
fi

iptables -F OUTPUT
iptables -A OUTPUT -o lo -j ACCEPT
iptables -A OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
# DNS, so the allowlist below (and the agent's own runtime lookups) can resolve.
iptables -A OUTPUT -p udp --dport 53 -j ACCEPT
iptables -A OUTPUT -p tcp --dport 53 -j ACCEPT

# Permit the resolved IPv4 addresses of each allowlisted domain.
IFS=','
for d in $allow; do
  d="$(printf '%s' "$d" | tr -d '[:space:]')"
  [ -z "$d" ] && continue
  for ip in $(getent ahostsv4 "$d" | awk '{print $1}' | sort -u); do
    iptables -A OUTPUT -d "$ip" -j ACCEPT
  done
done
unset IFS

# Everything else outbound is rejected with a fast, visible error (not a hang).
iptables -A OUTPUT -j REJECT --reject-with icmp-port-unreachable
FW
RUN chmod +x /usr/local/bin/sandbox-egress-setup

# sandbox-firewall is the container entrypoint used in allowlist mode. It runs as
# root, programs the firewall (failing closed if that errors — better to refuse
# than to run without the requested allowlist), then drops to the intended user
# (SANDBOX_RUN_AS, e.g. `sandbox`) before exec'ing the guest command. With no
# SANDBOX_EGRESS_ALLOW set it is a transparent passthrough.
RUN cat > /usr/local/bin/sandbox-firewall <<'EP'
#!/usr/bin/env bash
set -euo pipefail

if [ -n "${SANDBOX_EGRESS_ALLOW:-}" ]; then
  if ! sandbox-egress-setup; then
    echo "sandbox-cli: egress firewall setup failed; refusing to run without the requested allowlist" >&2
    exit 1
  fi
fi

user="${SANDBOX_RUN_AS:-}"
if [ -n "$user" ] && [ "$user" != "root" ] && [ "$(id -u)" = "0" ]; then
  uid="$(id -u "$user")"
  gid="$(id -g "$user")"
  exec setpriv --reuid "$uid" --regid "$gid" --init-groups -- "$@"
fi
exec "$@"
EP
RUN chmod +x /usr/local/bin/sandbox-firewall

# Isolated, non-root user whose home IS /sandbox/home — set as the passwd home
# (-d) so $HOME and getpwuid agree and there is no stray /home/sandbox. Nothing
# here is host-connected: the only host mount is /workspace, injected at run
# time. No explicit UID: the base node image already uses UID 1000, so we let
# useradd pick a free one.
RUN useradd -m -d /sandbox/home -s /bin/bash sandbox \
    && mkdir -p /workspace \
    # Pre-create the package-manager cache dirs so that when `--cache` mounts a
    # fresh named volume over one, docker initializes the volume from this
    # sandbox-owned directory and the non-root user can write to it.
    && mkdir -p /sandbox/home/.npm \
       /sandbox/home/.cache/pip \
       /sandbox/home/.cargo/registry \
       /sandbox/home/go/pkg/mod \
       /sandbox/home/.cache/yarn \
    && chown -R sandbox:sandbox /sandbox/home /workspace

ENV HOME=/sandbox/home
# Prefer a self-updating Claude install in the persisted HOME over the baked one.
ENV PATH=/sandbox/home/.local/bin:${PATH}
WORKDIR /workspace

# No USER is set here, so the image itself defaults to root — but sandbox-cli
# runs as the non-root `sandbox` user by default (config.Default), which is safe
# on macOS Docker Desktop where bind ownership is virtualized. Override at run
# time with `--user root` (or `--user uid:gid`).
