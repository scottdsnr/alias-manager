#!/usr/bin/env sh
# Install alias-manager into ~/.local/bin.
#
#   curl -fsSL https://raw.githubusercontent.com/scottdsnr/alias-manager/master/scripts/install.sh | sh
#
# Env:
#   VERSION   tag to install (default: latest release)
#   BIN_DIR   install directory (default: ~/.local/bin)
set -eu

REPO="scottdsnr/alias-manager"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"

say() { printf '%s\n' "$*"; }
die() { printf 'install: %s\n' "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required"; }
need tar

if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -qO- "$1"; }
else
  die "curl or wget is required"
fi

case "$(uname -s)" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *) die "unsupported OS: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

version="${VERSION:-}"
if [ -z "$version" ]; then
  version=$(fetch "https://api.github.com/repos/$REPO/releases/latest" \
    | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
    | head -n1)
  [ -n "$version" ] || die "could not determine the latest release"
fi

asset="alias-manager_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/$version/$asset"

say "Installing alias-manager $version ($os/$arch)"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

fetch "$url" > "$tmp/$asset" || die "download failed: $url"
tar -xzf "$tmp/$asset" -C "$tmp" || die "could not extract $asset"
[ -f "$tmp/alias-manager" ] || die "archive did not contain alias-manager"

mkdir -p "$BIN_DIR"
install -m 0755 "$tmp/alias-manager" "$BIN_DIR/alias-manager" 2>/dev/null \
  || { cp "$tmp/alias-manager" "$BIN_DIR/alias-manager" && chmod 0755 "$BIN_DIR/alias-manager"; }

say "Installed $BIN_DIR/alias-manager"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *)
    say ""
    say "$BIN_DIR is not on your PATH. Add this to your shell rc file:"
    say "    export PATH=\"\$PATH:$BIN_DIR\""
    ;;
esac

say "Run 'alias-manager' to get started, or 'alias-manager -update' later."
