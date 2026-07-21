#!/usr/bin/env python3
"""Install sandbox-cli: pick the right release binary for this machine and drop
it in the user's home (no root, no package manager).

    curl -fsSL https://raw.githubusercontent.com/Aegmis/sandbox-cli/main/install.py | python3 -

Options (when run as a file, e.g. `python3 install.py --version 0.0.1beta.1`):
    --version VER   install a specific release (default: latest)
    --dest DIR      install directory (default: ~/.local/bin, or
                    %LOCALAPPDATA%\\Programs\\sandbox-cli on Windows)
    --token TOK     GitHub token, for a private repo (or set GITHUB_TOKEN)

Standard library only; needs Python 3.8+.
"""

import argparse
import hashlib
import json
import os
import platform
import shutil
import stat
import sys
import tempfile
import urllib.error
import urllib.request
from pathlib import Path

REPO = "Aegmis/sandbox-cli"
BINARY = "sandbox-cli"
API = f"https://api.github.com/repos/{REPO}"
DOWNLOAD = f"https://github.com/{REPO}/releases/download"


def detect_platform():
    """Map this machine to the (os, arch) used in release asset names."""
    system = platform.system().lower()
    os_name = {"linux": "linux", "darwin": "darwin", "windows": "windows"}.get(system)
    if os_name is None:
        die(f"unsupported operating system: {platform.system()}")

    machine = platform.machine().lower()
    arch = {
        "x86_64": "amd64", "amd64": "amd64",
        "aarch64": "arm64", "arm64": "arm64",
    }.get(machine)
    if arch is None:
        die(f"unsupported architecture: {platform.machine()}")

    return os_name, arch


def default_dest(os_name):
    if os_name == "windows":
        base = os.environ.get("LOCALAPPDATA") or (Path.home() / "AppData" / "Local")
        return Path(base) / "Programs" / BINARY
    return Path.home() / ".local" / "bin"


def http_get(url, token=None, accept=None):
    req = urllib.request.Request(url)
    req.add_header("User-Agent", f"{BINARY}-installer")
    if accept:
        req.add_header("Accept", accept)
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(req) as resp:
            return resp.read()
    except urllib.error.HTTPError as e:
        if e.code in (401, 403, 404):
            die(
                f"cannot fetch {url} (HTTP {e.code}).\n"
                "If the repository is private, pass --token or set GITHUB_TOKEN."
            )
        die(f"cannot fetch {url} (HTTP {e.code})")
    except urllib.error.URLError as e:
        die(f"network error fetching {url}: {e.reason}")


def latest_version(token):
    data = json.loads(http_get(f"{API}/releases/latest", token))
    tag = data.get("tag_name")
    if not tag:
        die("could not determine the latest release tag")
    return tag


def verify_checksum(path, asset, sums_text):
    """Check the download against the release's SHA256SUMS, when published."""
    expected = None
    for line in sums_text.splitlines():
        parts = line.split()
        if len(parts) == 2 and parts[1].lstrip("*") == asset:
            expected = parts[0]
            break
    if expected is None:
        print(f"  ! {asset} not listed in SHA256SUMS; skipping verification")
        return

    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    actual = h.hexdigest()
    if actual != expected:
        die(f"checksum mismatch for {asset}\n  expected {expected}\n  actual   {actual}")
    print("  checksum ok")


def die(msg):
    print(f"error: {msg}", file=sys.stderr)
    sys.exit(1)


def main():
    ap = argparse.ArgumentParser(description="Install sandbox-cli into your home directory.")
    ap.add_argument("--version", help="release to install (default: latest)")
    ap.add_argument("--dest", help="install directory")
    ap.add_argument("--token", help="GitHub token for a private repo")
    args = ap.parse_args()

    token = args.token or os.environ.get("GITHUB_TOKEN") or os.environ.get("GH_TOKEN")

    os_name, arch = detect_platform()
    version = args.version or latest_version(token)
    ext = ".exe" if os_name == "windows" else ""
    asset = f"{BINARY}_{version}_{os_name}_{arch}{ext}"

    dest_dir = Path(args.dest).expanduser() if args.dest else default_dest(os_name)
    target = dest_dir / (BINARY + ext)

    print(f"sandbox-cli {version} -> {target}")
    print(f"  platform: {os_name}/{arch}")

    with tempfile.TemporaryDirectory() as tmp:
        tmp_bin = Path(tmp) / asset
        print(f"  downloading {asset}")
        tmp_bin.write_bytes(http_get(f"{DOWNLOAD}/{version}/{asset}", token,
                                     accept="application/octet-stream"))

        try:
            sums = http_get(f"{DOWNLOAD}/{version}/SHA256SUMS", token).decode()
            verify_checksum(tmp_bin, asset, sums)
        except SystemExit:
            print("  ! SHA256SUMS not published for this release; skipping verification")

        dest_dir.mkdir(parents=True, exist_ok=True)
        # Copy then chmod then replace, so an in-use binary is swapped atomically.
        staged = dest_dir / f".{BINARY}.new"
        shutil.copyfile(tmp_bin, staged)
        staged.chmod(staged.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
        os.replace(staged, target)

    print(f"installed {target}")

    path_entries = os.environ.get("PATH", "").split(os.pathsep)
    if str(dest_dir) not in path_entries:
        print(f"\nNote: {dest_dir} is not on your PATH. Add it:")
        if os_name == "windows":
            print(f'  setx PATH "%PATH%;{dest_dir}"')
        else:
            shell_rc = "~/.zshrc" if os.environ.get("SHELL", "").endswith("zsh") else "~/.bashrc"
            print(f'  echo \'export PATH="{dest_dir}:$PATH"\' >> {shell_rc} && exec $SHELL')
    else:
        print(f"Run: {BINARY} --help")


if __name__ == "__main__":
    main()
