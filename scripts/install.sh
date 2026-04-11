#!/bin/sh
# okdctl installer
# ---------------------
# Downloads the latest (or a pinned) okdctl release from GitHub,
# verifies its SHA256 against the published SHA256SUMS file, and installs
# the binary to /usr/local/bin (or $INSTALL_DIR if set).
#
# Usage:
#   curl -sSfL https://raw.githubusercontent.com/qxtaiba/okdctl/main/scripts/install.sh | sh
#
# Environment variables:
#   VERSION      - pin to a specific release, e.g. VERSION=v0.1.0 (default: latest)
#   INSTALL_DIR  - where to put the binary (default: /usr/local/bin)
#   INSECURE     - set to "1" to skip checksum verification (NOT recommended)
#
# Requires: curl, sha256sum (or shasum -a 256), tar (or unzip on macOS).

set -eu

REPO="qxtaiba/okdctl"
BINARY="okdctl"
VERSION="${VERSION:-}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
INSECURE="${INSECURE:-}"

red()   { printf '\033[31m%s\033[0m\n' "$*" >&2; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
info()  { printf '  %s\n' "$*"; }
die()   { red "error: $*"; exit 1; }

require() {
    command -v "$1" >/dev/null 2>&1 || die "$1 is required but not installed"
}

require curl
require tar

# Detect sha256 tool (linux: sha256sum, macOS: shasum -a 256)
if command -v sha256sum >/dev/null 2>&1; then
    SHA_CMD="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
    SHA_CMD="shasum -a 256"
else
    [ -n "$INSECURE" ] || die "sha256sum or shasum -a 256 is required (or set INSECURE=1 to skip)"
    SHA_CMD=""
fi

# Detect OS and arch
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    linux)  OS="Linux" ;;
    darwin) OS="Darwin" ;;
    *) die "unsupported OS: $OS (supported: linux, darwin)" ;;
esac

ARCH=$(uname -m)
case "$ARCH" in
    x86_64 | amd64) ARCH="x86_64" ;;
    aarch64 | arm64) ARCH="arm64" ;;
    *) die "unsupported arch: $ARCH (supported: x86_64, arm64)" ;;
esac

# Resolve latest version if not pinned. Pattern adapted from get.helm.sh
# (sed -n + capture group), more robust than grep | head | cut against
# JSON key reordering or whitespace variation in the API response.
if [ -z "$VERSION" ]; then
    info "resolving latest release..."
    VERSION=$(curl -sSfL "https://api.github.com/repos/$REPO/releases/latest" |
        sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' |
        head -1)
    [ -n "$VERSION" ] || die "failed to resolve latest release from GitHub API"
    info "latest: $VERSION"
fi

# Determine archive format: darwin=zip, linux=tar.gz
VERSION_NOPREFIX="${VERSION#v}"
case "$OS" in
    Darwin) ARCHIVE_EXT="zip" ;;
    Linux)  ARCHIVE_EXT="tar.gz" ;;
esac

ARCHIVE_NAME="${BINARY}_${VERSION_NOPREFIX}_${OS}_${ARCH}.${ARCHIVE_EXT}"
BASE_URL="https://github.com/$REPO/releases/download/$VERSION"
ARCHIVE_URL="$BASE_URL/$ARCHIVE_NAME"
SHA_URL="$BASE_URL/SHA256SUMS"

# Download into a temporary directory that gets cleaned up on exit.
TMP=$(mktemp -d 2>/dev/null || mktemp -d -t "$BINARY")
trap 'rm -rf "$TMP"' EXIT INT TERM

info "downloading $ARCHIVE_NAME"
curl -sSfL -o "$TMP/$ARCHIVE_NAME" "$ARCHIVE_URL" ||
    die "failed to download $ARCHIVE_URL"

# Verify SHA256 unless explicitly skipped.
if [ -z "$INSECURE" ] && [ -n "$SHA_CMD" ]; then
    info "verifying SHA256"
    curl -sSfL -o "$TMP/SHA256SUMS" "$SHA_URL" ||
        die "failed to download SHA256SUMS from $SHA_URL"
    EXPECTED=$(grep " $ARCHIVE_NAME\$" "$TMP/SHA256SUMS" | awk '{print $1}')
    [ -n "$EXPECTED" ] || die "no checksum found for $ARCHIVE_NAME in SHA256SUMS"
    ACTUAL=$($SHA_CMD "$TMP/$ARCHIVE_NAME" | awk '{print $1}')
    [ "$EXPECTED" = "$ACTUAL" ] ||
        die "checksum mismatch: expected $EXPECTED, got $ACTUAL"
    info "checksum verified"
fi

# Extract the archive.
info "extracting"
cd "$TMP"
case "$ARCHIVE_EXT" in
    zip)
        command -v unzip >/dev/null 2>&1 ||
            die "unzip is required to extract darwin archives"
        unzip -q "$ARCHIVE_NAME"
        ;;
    tar.gz)
        tar -xzf "$ARCHIVE_NAME"
        ;;
esac

[ -f "$BINARY" ] || die "$BINARY not found in archive"

# Install. If the install dir is not writable by the current user, try sudo.
info "installing to $INSTALL_DIR/$BINARY"
if [ -w "$INSTALL_DIR" ]; then
    install -m 0755 "$BINARY" "$INSTALL_DIR/$BINARY"
elif command -v sudo >/dev/null 2>&1; then
    sudo install -m 0755 "$BINARY" "$INSTALL_DIR/$BINARY"
else
    die "$INSTALL_DIR is not writable and sudo is not available"
fi

green "okdctl $VERSION installed to $INSTALL_DIR/$BINARY"
info "run 'okdctl doctor' to verify your environment"
