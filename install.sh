#!/usr/bin/env sh
# install.sh — build charon from source and install it onto your PATH.
#
# Usage:
#   ./install.sh                 # install to ~/.local/bin (no sudo)
#   PREFIX=/usr/local ./install.sh   # install to /usr/local/bin (may need sudo)
#
# Requires: Go 1.24+ and a POSIX shell.

set -eu

BINARY="charon"
PKG="./cmd/charon"
MIN_GO_MINOR=24
PREFIX="${PREFIX:-$HOME/.local}"
BINDIR="$PREFIX/bin"

# Run from the script's own directory so relative paths resolve.
cd "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

info() { printf '\033[36m==>\033[0m %s\n' "$1"; }
warn() { printf '\033[33mwarning:\033[0m %s\n' "$1" >&2; }
die()  { printf '\033[31merror:\033[0m %s\n' "$1" >&2; exit 1; }

# 1. Check Go is present and new enough.
command -v go >/dev/null 2>&1 || die "Go is not installed. Install Go 1.$MIN_GO_MINOR+ from https://go.dev/dl/"
GO_VER=$(go env GOVERSION 2>/dev/null | sed 's/^go//')
GO_MINOR=$(printf '%s' "$GO_VER" | cut -d. -f2)
case "$GO_MINOR" in
  ''|*[!0-9]*) warn "could not parse Go version ($GO_VER); continuing" ;;
  *) [ "$GO_MINOR" -ge "$MIN_GO_MINOR" ] || die "Go 1.$MIN_GO_MINOR+ required, found $GO_VER" ;;
esac
info "Using Go $GO_VER"

# 2. Determine a version string from git if available.
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)

# 3. Build.
info "Building $BINARY $VERSION ..."
CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$VERSION" -o "$BINARY" "$PKG"

# 4. Install.
info "Installing to $BINDIR ..."
mkdir -p "$BINDIR"
install -m 0755 "$BINARY" "$BINDIR/$BINARY"
rm -f "$BINARY"

# 5. PATH check + next steps.
info "Installed: $BINDIR/$BINARY"
case ":$PATH:" in
  *":$BINDIR:"*) : ;;
  *)
    warn "$BINDIR is not on your PATH."
    printf '  Add this to your shell profile (~/.zshrc or ~/.bashrc):\n'
    printf '    export PATH="%s:$PATH"\n' "$BINDIR"
    ;;
esac

printf '\nDone. Run:\n  %s          # interactive menu\n  %s status   # show detected tools\n' "$BINARY" "$BINARY"
