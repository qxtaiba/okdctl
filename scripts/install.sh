#!/bin/sh
# okdctl installer
# ---------------------
# Downloads the latest (or a pinned) okdctl release from GitHub,
# verifies its SHA256 against the published SHA256SUMS file, verifies the
# cosign signature on SHA256SUMS when cosign is available, and installs the
# binary to /usr/local/bin (or $INSTALL_DIR if set).
#
# Usage:
#   curl -sSfL https://raw.githubusercontent.com/qxtaiba/okdctl/main/scripts/install.sh | sh
#
# Environment variables:
#   VERSION      - pin to a specific release, e.g. VERSION=v0.1.0 (default: latest)
#   INSTALL_DIR  - where to put the binary (default: /usr/local/bin)
#   INSECURE     - set to "1" to skip checksum verification (NOT recommended)
#
# Requires: curl, tar, sha256sum. Optionally: cosign (highly recommended).

set -eu

REPO="qxtaiba/okdctl"
BINARY="okdctl"
VERSION="${VERSION:-}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
INSECURE="${INSECURE:-}"

if [ -n "$INSECURE" ]; then
    printf '\033[31mWARNING: INSECURE=1 is set — SHA256 and cosign signature verification SKIPPED.\033[0m\n' >&2
    printf '\033[31m         A compromised GitHub release or CDN can substitute arbitrary binaries.\033[0m\n' >&2
    printf '\033[31m         Unset INSECURE to re-enable verification.\033[0m\n' >&2
fi

red()   { printf '\033[31m%s\033[0m\n' "$*" >&2; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
info()  { printf '  %s\n' "$*"; }
die()   { red "error: $*"; exit 1; }

require() {
    command -v "$1" >/dev/null 2>&1 || die "$1 is required but not installed"
}

require curl
require tar

if command -v sha256sum >/dev/null 2>&1; then
    SHA_CMD="sha256sum"
else
    [ -n "$INSECURE" ] || die "sha256sum is required (or set INSECURE=1 to skip checksum verification)"
    SHA_CMD=""
fi

# okdctl is Linux-only. Refuse to install on anything else.
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    linux) OS="Linux" ;;
    *) die "unsupported OS: $OS — okdctl runs on Linux only (the deploy phase needs dnf/apt, systemd, firewall-cmd, nmcli)" ;;
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

VERSION_NOPREFIX="${VERSION#v}"
ARCHIVE_NAME="${BINARY}_${VERSION_NOPREFIX}_${OS}_${ARCH}.tar.gz"
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
    info "downloading SHA256SUMS"
    curl -sSfL -o "$TMP/SHA256SUMS" "$SHA_URL" ||
        die "failed to download SHA256SUMS from $SHA_URL"

    # Cosign verify-blob against the sigstore-published signature.
    # goreleaser publishes SHA256SUMS.sig + SHA256SUMS.pem for every
    # release — verifying these closes the window where an attacker who
    # controls release-asset upload can swap both archive and SHA256SUMS.
    if command -v cosign >/dev/null 2>&1; then
        info "verifying cosign signature on SHA256SUMS"
        curl -sSfL -o "$TMP/SHA256SUMS.sig" "$BASE_URL/SHA256SUMS.sig" ||
            die "failed to download SHA256SUMS.sig (release missing signature? rerun with INSECURE=1 if you accept the risk)"
        curl -sSfL -o "$TMP/SHA256SUMS.pem" "$BASE_URL/SHA256SUMS.pem" ||
            die "failed to download SHA256SUMS.pem"
        COSIGN_EXPERIMENTAL=1 cosign verify-blob \
            --certificate="$TMP/SHA256SUMS.pem" \
            --signature="$TMP/SHA256SUMS.sig" \
            --certificate-identity-regexp='https://github\.com/qxtaiba/okdctl/' \
            --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
            "$TMP/SHA256SUMS" >/dev/null 2>&1 ||
            die "cosign signature verification failed on SHA256SUMS"
        info "cosign signature verified"
    else
        info "cosign not installed — skipping signature verification (checksum still enforced)"
        info "install cosign from https://docs.sigstore.dev/system_config/installation/ to enable signature verification"
    fi

    info "verifying SHA256"
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
tar -xzf "$ARCHIVE_NAME"

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
