#!/usr/bin/env bash
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
#   VERSION       - pin to a specific release, e.g. VERSION=v0.1.0 (default: latest)
#   INSTALL_DIR   - where to put the binary (default: /usr/local/bin)
#   INSECURE      - set to "1" to skip SHA256 checksum verification (NOT recommended);
#                   cosign signature verification still runs when cosign is installed.
#   GITHUB_TOKEN  - bearer token injected when resolving the latest release;
#                   lifts the GitHub API rate limit from 60 to 5 000 req/hr/IP,
#                   which matters on shared CI runners with many co-tenants.
#
# Requires: bash, curl, tar, sha256sum. Optionally: cosign (highly recommended).

set -euo pipefail

REPO="qxtaiba/okdctl"
BINARY="okdctl"
VERSION="${VERSION:-}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
INSECURE="${INSECURE:-}"

red()   { printf '\033[31m%s\033[0m\n' "$*" >&2; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
info()  { printf '  %s\n' "$*"; }
die()   { red "error: $*"; exit 1; }

# curl_safe wraps curl with hardened defaults: HTTPS-only, TLS 1.2 floor,
# connect + transfer timeouts, and two retries on connection refusal.
curl_safe() { curl --proto '=https' --tlsv1.2 --connect-timeout 10 --max-time 120 --retry 2 --retry-connrefused "$@"; }

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

COSIGN_CMD=""
if command -v cosign >/dev/null 2>&1; then
    COSIGN_CMD="cosign"
fi

if [ -n "$INSECURE" ]; then
    if [ -n "$COSIGN_CMD" ]; then
        printf '\033[31mWARNING: INSECURE=1 is set — SHA256 verification SKIPPED (cosign signature verification still active).\033[0m\n' >&2
    else
        printf '\033[31mWARNING: INSECURE=1 is set — SHA256 and cosign signature verification SKIPPED.\033[0m\n' >&2
    fi
    printf '\033[31m         A compromised GitHub release or CDN can substitute arbitrary binaries.\033[0m\n' >&2
    printf '\033[31m         Unset INSECURE to re-enable SHA256 verification.\033[0m\n' >&2
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
    # Inject a bearer token when available to lift the GitHub API rate limit
    # from 60 to 5 000 req/hr per IP — relevant on shared CI runners.
    _gh_auth_header=()
    if [ -n "${GITHUB_TOKEN:-}" ]; then
        _gh_auth_header=(-H "Authorization: Bearer $GITHUB_TOKEN")
    fi
    VERSION=$(curl -sSfL --proto '=https' --tlsv1.2 --connect-timeout 10 --max-time 30 \
        "${_gh_auth_header[@]}" \
        "https://api.github.com/repos/$REPO/releases/latest" |
        sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' |
        head -1)
    [ -n "$VERSION" ] || die "failed to resolve latest release from GitHub API; pin VERSION=vX.Y.Z or set GITHUB_TOKEN to avoid rate-limiting"
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
curl_safe -sSfL -o "$TMP/$ARCHIVE_NAME" "$ARCHIVE_URL" ||
    die "failed to download $ARCHIVE_URL"

# Download SHA256SUMS when at least one verification layer will consume it.
if [ -n "$COSIGN_CMD" ] || { [ -z "$INSECURE" ] && [ -n "$SHA_CMD" ]; }; then
    info "downloading SHA256SUMS"
    curl_safe -sSfL -o "$TMP/SHA256SUMS" "$SHA_URL" ||
        die "failed to download SHA256SUMS from $SHA_URL"
fi

# Cosign verify-blob against the sigstore-published signature; runs whenever
# cosign is present — independent of INSECURE so the stronger sigstore
# guarantee is not silently dropped by the sha256-skip flag.
if [ -n "$COSIGN_CMD" ]; then
    info "verifying cosign signature on SHA256SUMS"
    curl_safe -sSfL -o "$TMP/SHA256SUMS.sig" "$BASE_URL/SHA256SUMS.sig" ||
        die "failed to download SHA256SUMS.sig (release missing signature? uninstall cosign if you accept the risk)"
    curl_safe -sSfL -o "$TMP/SHA256SUMS.pem" "$BASE_URL/SHA256SUMS.pem" ||
        die "failed to download SHA256SUMS.pem"
    # stderr is intentionally passed through — on verification failure the
    # user needs to see cosign's diagnostic (cert identity, OIDC issuer,
    # signature mismatch) rather than a bare "verification failed".
    COSIGN_EXPERIMENTAL=1 cosign verify-blob \
        --certificate="$TMP/SHA256SUMS.pem" \
        --signature="$TMP/SHA256SUMS.sig" \
        --certificate-identity-regexp='https://github\.com/qxtaiba/okdctl/' \
        --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
        "$TMP/SHA256SUMS" >/dev/null ||
        die "cosign signature verification failed on SHA256SUMS"
    info "cosign signature verified"
else
    info "cosign not installed — skipping signature verification"
    info "install cosign from https://docs.sigstore.dev/system_config/installation/ to enable signature verification"
fi

# Verify SHA256 unless explicitly skipped.
if [ -z "$INSECURE" ] && [ -n "$SHA_CMD" ]; then
    info "verifying SHA256"
    # awk field-equality (not grep) avoids treating '.' in the filename as a
    # regex wildcard. Cosign already protects SHA256SUMS integrity so this
    # is defense-in-depth rather than a standalone guard.
    EXPECTED=$(awk -v name="$ARCHIVE_NAME" '$2 == name || $2 == "*"name {print $1}' "$TMP/SHA256SUMS")
    [ -n "$EXPECTED" ] || die "no checksum found for $ARCHIVE_NAME in SHA256SUMS"
    ACTUAL=$($SHA_CMD "$TMP/$ARCHIVE_NAME" | awk '{print $1}')
    [ "$EXPECTED" = "$ACTUAL" ] ||
        die "checksum mismatch: expected $EXPECTED, got $ACTUAL"
    info "checksum verified"
fi

# Extract the archive. --no-same-owner and --no-same-permissions harden
# against a release tarball that encodes unexpected ownership. The cosign
# + sha256 verification above is the primary guard; these are defense-in-depth.
info "extracting"
cd "$TMP"
tar --no-same-owner --no-same-permissions -xzf "$ARCHIVE_NAME"

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
