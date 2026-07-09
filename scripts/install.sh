#!/usr/bin/env sh
# install.sh — download a prebuilt charon binary and put it on your PATH.
#
# Quick install (Linux & macOS, no Go required):
#   curl -fsSL https://github.com/mingtheanlay/charon/releases/latest/download/install.sh | sh
#
# Options (environment variables):
#   PREFIX=/usr/local   install under <PREFIX>/bin instead of ~/.local (may need sudo)
#   VERSION=v1.2.3      install a specific release instead of the latest
#
# Requires: curl (or wget), tar, and sha256sum or shasum.

set -eu

REPO="mingtheanlay/charon"
BINARY="charon"
PREFIX="${PREFIX:-$HOME/.local}"
BINDIR="$PREFIX/bin"
VERSION="${VERSION:-latest}"

# --- print banner -------------------------------------------------------------
printf '\033[36m'
printf ' ██████╗██╗  ██╗ █████╗ ██████╗  ██████╗ ███╗   ██╗\n'
printf '██╔════╝██║  ██║██╔══██╗██╔══██╗██╔═══██╗████╗  ██║\n'
printf '██║     ███████║███████║██████╔╝██║   ██║██╔██╗ ██║\n'
printf '██║     ██╔══██║██╔══██║██╔══██╗██║   ██║██║╚██╗██║\n'
printf '╚██████╗██║  ██║██║  ██║██║  ██║╚██████╔╝██║ ╚████║\n'
printf ' ╚═════╝╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═══╝\n'
printf '\033[0m'
printf '  Ferrying your AI tools between saved profiles\n\n'

info() { printf '\033[36m==>\033[0m %s\n' "$1"; }
warn() { printf '\033[33mwarning:\033[0m %s\n' "$1" >&2; }
die()  { printf '\033[31merror:\033[0m %s\n' "$1" >&2; exit 1; }

# --- pick a downloader -------------------------------------------------------
if command -v curl >/dev/null 2>&1; then
  dl() { curl -fSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  dl() { wget -qO "$2" "$1"; }
else
  die "need curl or wget to download charon"
fi
command -v tar >/dev/null 2>&1 || die "tar is required"

# --- detect OS and architecture ---------------------------------------------
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux)  os=linux ;;
  darwin) os=darwin ;;
  *) die "unsupported OS: $os (only linux and darwin have prebuilt binaries)" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64)  arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "unsupported architecture: $arch" ;;
esac

archive="${BINARY}_${os}_${arch}.tar.gz"

# --- resolve the release URL -------------------------------------------------
# GitHub redirects .../releases/latest/download/<asset> to the newest tag.
if [ "$VERSION" = "latest" ]; then
  base="https://github.com/$REPO/releases/latest/download"
else
  base="https://github.com/$REPO/releases/download/$VERSION"
fi

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t charon)
trap 'rm -rf "$tmp"' EXIT INT TERM

info "Downloading $archive ($VERSION) ..."
dl "$base/$archive" "$tmp/$archive" || die "download failed — check the version/platform has a release asset"

# --- verify the checksum (best effort; warn if the list is unavailable) ------
if dl "$base/checksums.txt" "$tmp/checksums.txt" 2>/dev/null; then
  info "Verifying checksum ..."
  expected=$(grep " $archive\$" "$tmp/checksums.txt" | awk '{print $1}')
  [ -n "$expected" ] || die "no checksum listed for $archive"
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$tmp/$archive" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
  else
    warn "no sha256sum/shasum found; skipping checksum verification"
    actual="$expected"
  fi
  [ "$actual" = "$expected" ] || die "checksum mismatch for $archive (expected $expected, got $actual)"
else
  warn "checksums.txt not available; skipping verification"
fi

# --- unpack and install ------------------------------------------------------
info "Unpacking ..."
tar -xzf "$tmp/$archive" -C "$tmp"
[ -f "$tmp/$BINARY" ] || die "archive did not contain a '$BINARY' binary"

info "Installing to $BINDIR ..."
if ! mkdir -p "$BINDIR" 2>/dev/null || ! install -m 0755 "$tmp/$BINARY" "$BINDIR/$BINARY" 2>/dev/null; then
  warn "could not write to $BINDIR without elevated permissions; retrying with sudo"
  sudo mkdir -p "$BINDIR"
  sudo install -m 0755 "$tmp/$BINARY" "$BINDIR/$BINARY"
fi

info "Installed: $BINDIR/$BINARY"

# --- PATH check --------------------------------------------------------------
case ":$PATH:" in
  *":$BINDIR:"*) : ;;
  *)
    warn "$BINDIR is not on your PATH."
    printf '  Add this to your shell profile (~/.bashrc or ~/.zshrc):\n'
    printf '    export PATH="%s:$PATH"\n' "$BINDIR"
    ;;
esac

printf '\nDone. Run:\n  %s          # interactive menu\n  %s status   # show detected tools\n' "$BINARY" "$BINARY"
