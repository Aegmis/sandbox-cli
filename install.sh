#!/bin/sh
# Install sandbox-cli: pick the right release archive for this machine and put
# the binary in the user's home. No root, no package manager.
#
#   curl -fsSL https://raw.githubusercontent.com/Amitgb14/sandbox-cli/main/install.sh | sh
#
# Options (when run as a file, e.g. `sh install.sh --version 0.0.1beta.2`):
#   --version VER   install a specific release        (default: latest)
#   --dest DIR      install directory                 (default: ~/.local/bin)
#   --token TOK     GitHub token for a private repo   (or set GITHUB_TOKEN)
#   --uninstall     remove the binary, then report what else is left behind
#   --purge         with --uninstall: also delete ~/.config/sandbox (agent
#                   logins!) and the sandbox Docker images and cache volumes
#
# POSIX sh; needs curl or wget, plus tar.

set -eu

REPO="Amitgb14/sandbox-cli"
BINARY="sandbox-cli"
VERSION=""
DEST="${HOME}/.local/bin"
TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
UNINSTALL=0
PURGE=0

die() { printf 'error: %s\n' "$*" >&2; exit 1; }
info() { printf '%s\n' "$*"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --version)   VERSION="${2:-}"; shift 2 ;;
    --dest)      DEST="${2:-}"; shift 2 ;;
    --token)     TOKEN="${2:-}"; shift 2 ;;
    --uninstall) UNINSTALL=1; shift ;;
    --purge)     PURGE=1; shift ;;
    -h|--help)   sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done

# ---- uninstall --------------------------------------------------------------
# Deliberately conservative: the binary goes, everything else is only listed
# unless --purge is given. ~/.config/sandbox holds your agent logins, so
# deleting it silently would sign you out of Claude/Codex with no warning.
if [ "$UNINSTALL" = 1 ]; then
  cfg="${XDG_CONFIG_HOME:-${HOME}/.config}/sandbox"

  # Docker may be absent or not running; never let that fail the uninstall.
  sandbox_images() {
    command -v docker >/dev/null 2>&1 || return 0
    docker images --filter reference='sandbox-base' -q 2>/dev/null | sort -u
  }
  sandbox_volumes() {
    command -v docker >/dev/null 2>&1 || return 0
    docker volume ls --filter name='sandbox-cache-' -q 2>/dev/null
  }

  removed=0
  for d in "$DEST" "${HOME}/.local/bin" /usr/local/bin; do
    if [ -f "${d}/${BINARY}" ]; then
      rm -f "${d}/${BINARY}"
      info "removed ${d}/${BINARY}"
      removed=1
    fi
  done
  if [ "$removed" = 0 ]; then
    info "no ${BINARY} binary found in ${DEST}, ~/.local/bin or /usr/local/bin"
  fi

  imgs=$(sandbox_images)
  vols=$(sandbox_volumes)

  if [ "$PURGE" = 1 ]; then
    if [ -d "$cfg" ]; then
      rm -rf "$cfg"
      info "removed ${cfg}  (config + agent logins)"
    fi
    if [ -n "$imgs" ]; then
      # Unquoted on purpose: one id per line, split into separate arguments.
      docker rmi -f $imgs >/dev/null 2>&1 || true
      info "removed sandbox-base image(s)"
    fi
    if [ -n "$vols" ]; then
      docker volume rm $vols >/dev/null 2>&1 || true
      info "removed sandbox-cache-* volume(s)"
    fi
    info "purge complete"
  else
    # Only print the "left behind" report when something actually is.
    if [ -d "$cfg" ] || [ -n "$imgs" ] || [ -n "$vols" ]; then
      info ""
      info "Left in place — re-run with --uninstall --purge to delete these too:"
      # `|| true` on each: a failed test is an AND-OR list with status 1, which
      # `set -e` would otherwise treat as fatal and abort the report mid-way.
      [ -d "$cfg" ] && info "  ${cfg}  (config + agent logins)" || true
      [ -n "$imgs" ] && info "  sandbox-base image(s)      docker rmi \$(docker images -q sandbox-base)" || true
      [ -n "$vols" ] && info "  sandbox-cache-* volume(s)  docker volume rm \$(docker volume ls -q -f name=sandbox-cache-)" || true
    fi
    info ""
    info "Your projects and their .sandbox.yaml files are never touched."
  fi
  exit 0
fi

# ---- http helper (curl or wget) ---------------------------------------------
if command -v curl >/dev/null 2>&1; then
  fetch() { # fetch URL OUTFILE
    if [ -n "$TOKEN" ]; then
      curl -fsSL -H "Authorization: Bearer $TOKEN" -H "Accept: application/octet-stream" -o "$2" "$1"
    else
      curl -fsSL -o "$2" "$1"
    fi
  }
elif command -v wget >/dev/null 2>&1; then
  fetch() {
    if [ -n "$TOKEN" ]; then
      wget -q --header "Authorization: Bearer $TOKEN" --header "Accept: application/octet-stream" -O "$2" "$1"
    else
      wget -q -O "$2" "$1"
    fi
  }
else
  die "need curl or wget"
fi

# ---- detect platform --------------------------------------------------------
os=$(uname -s)
case "$os" in
  Linux)  OS=linux ;;
  Darwin) OS=darwin ;;
  MINGW*|MSYS*|CYGWIN*)
    die "Windows is not supported by this script; download the .zip from
  https://github.com/${REPO}/releases" ;;
  *) die "unsupported operating system: $os" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) die "unsupported architecture: $arch" ;;
esac

# ---- resolve version --------------------------------------------------------
TMP=$(mktemp -d)
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT INT TERM

if [ -z "$VERSION" ]; then
  # The releases list, newest first — not /releases/latest, which silently
  # excludes pre-releases and 404s when every release is one.
  fetch "https://api.github.com/repos/${REPO}/releases?per_page=1" "$TMP/rel.json" \
    || die "cannot reach the GitHub API.
  If the repository is private, pass --token or set GITHUB_TOKEN."
  VERSION=$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$TMP/rel.json" | head -1)
  [ -n "$VERSION" ] || die "no releases found for ${REPO}.
  See https://github.com/${REPO}/releases, or pass --version explicitly."
fi

ARCHIVE="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/${REPO}/releases/download/${VERSION}"

info "${BINARY} ${VERSION} -> ${DEST}/${BINARY}"
info "  platform: ${OS}/${ARCH}"

# ---- download ---------------------------------------------------------------
info "  downloading ${ARCHIVE}"
fetch "${BASE}/${ARCHIVE}" "$TMP/$ARCHIVE" || die "download failed: ${BASE}/${ARCHIVE}
  If the repository is private, pass --token or set GITHUB_TOKEN."

# ---- verify checksum --------------------------------------------------------
if fetch "${BASE}/checksums.txt" "$TMP/checksums.txt" 2>/dev/null; then
  expected=$(grep " ${ARCHIVE}\$" "$TMP/checksums.txt" | awk '{print $1}' | head -1)
  if [ -n "$expected" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      actual=$(sha256sum "$TMP/$ARCHIVE" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
      actual=$(shasum -a 256 "$TMP/$ARCHIVE" | awk '{print $1}')
    else
      actual=""
      info "  ! no sha256 tool found; skipping verification"
    fi
    if [ -n "$actual" ]; then
      [ "$actual" = "$expected" ] || die "checksum mismatch for ${ARCHIVE}
  expected ${expected}
  actual   ${actual}"
      info "  checksum ok"
    fi
  else
    info "  ! ${ARCHIVE} not listed in checksums.txt; skipping verification"
  fi
else
  info "  ! checksums.txt not published for this release; skipping verification"
fi

# ---- install ----------------------------------------------------------------
tar -xzf "$TMP/$ARCHIVE" -C "$TMP" "$BINARY" 2>/dev/null \
  || tar -xzf "$TMP/$ARCHIVE" -C "$TMP" \
  || die "could not extract ${ARCHIVE}"
[ -f "$TMP/$BINARY" ] || die "${BINARY} not found inside ${ARCHIVE}"

mkdir -p "$DEST"
chmod +x "$TMP/$BINARY"
# Stage then rename, so replacing a running binary is atomic.
mv "$TMP/$BINARY" "$DEST/.${BINARY}.new"
mv "$DEST/.${BINARY}.new" "$DEST/$BINARY"

info "installed ${DEST}/${BINARY}"

# ---- PATH hint --------------------------------------------------------------
case ":${PATH}:" in
  *":${DEST}:"*)
    info "Run: ${BINARY} --help" ;;
  *)
    case "${SHELL:-}" in
      */zsh) rc="~/.zshrc" ;;
      */fish) rc="~/.config/fish/config.fish" ;;
      *) rc="~/.bashrc" ;;
    esac
    printf '\nNote: %s is not on your PATH. Add it:\n' "$DEST"
    printf '  echo '\''export PATH="%s:$PATH"'\'' >> %s && exec $SHELL\n' "$DEST" "$rc" ;;
esac
