#!/usr/bin/env bash
# okdctl installer
# ---------------------
# Downloads the latest (or a pinned) okdctl release from GitHub,
# verifies its SHA256 against the published SHA256SUMS file, verifies the
# cosign signature on SHA256SUMS when cosign is available, and installs the
# binary to /usr/local/bin (or $INSTALL_DIR if set).
#
# Usage:
#   curl -sSfL https://raw.githubusercontent.com/qxtaiba/okdctl/develop/scripts/install.sh | bash
#
# Environment variables:
#   VERSION       - pin to a specific release, e.g. VERSION=v0.1.0 (default: latest)
#   INSTALL_DIR   - where to put the binary (default: /usr/local/bin)
#   INSECURE      - set to "1" to skip cosign signature verification when cosign is
#                   absent or when you explicitly accept sha256-only trust.
#   GITHUB_TOKEN  - bearer token injected when resolving the latest release;
#                   lifts the GitHub API rate limit from 60 to 5 000 req/hr/IP,
#                   which matters on shared CI runners with many co-tenants.
#
# Requires: bash, curl, tar, sha256sum. Optionally: cosign (highly recommended).
#
# Supply-chain trust layers (in order):
#   1. TLS to GitHub Releases — curl_safe enforces --proto =https and
#      --tlsv1.2; a downgrade or MITM is rejected at the transport layer.
#   2. Cosign on SHA256SUMS — sigstore keyless signature ties SHA256SUMS to
#      a specific GitHub Actions workflow run; a forged or substituted
#      SHA256SUMS file fails certificate-identity verification.
#   3. SHA256 on archive — the downloaded tarball is byte-compared against
#      the cosign-verified checksum; a corrupted or swapped archive is
#      rejected before any bytes land on the filesystem.
#   4. --no-same-permissions on tar extraction — drops extracted file modes
#      to the caller's umask (typically 0o755 for executables), preventing
#      a tarball that encodes setuid/setgid bits from elevating privileges
#      even when the sha256 check passes.
#   5. install -m 0755 final write — regardless of the mode produced by
#      step 4, the binary is written to $INSTALL_DIR with an explicit
#      0755 mode, so the installed binary always has a known, safe mode.

set -euo pipefail

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

main() {
    REPO="qxtaiba/okdctl"
    BINARY="okdctl"
    VERSION="${VERSION:-}"
    INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
    INSECURE="${INSECURE:-}"

    require curl
    require tar

    # sha256sum ships with coreutils on every supported Linux distro; refuse
    # early rather than offering an env-var bypass when it is absent.
    if command -v sha256sum >/dev/null 2>&1; then
        SHA_CMD="sha256sum"
    else
        die "sha256sum is required but not installed; install coreutils (e.g. apt-get install coreutils, dnf install coreutils)"
    fi

    COSIGN_CMD=""
    if command -v cosign >/dev/null 2>&1; then
        COSIGN_CMD="cosign"
    fi

    if [ -z "$COSIGN_CMD" ]; then
        if [ -z "$INSECURE" ]; then
            die "cosign is required but not installed; install cosign (https://docs.sigstore.dev/cosign/installation/) or set INSECURE=1 to accept sha256-only verification"
        fi
        red "WARNING: cosign not installed — falling back to sha256-only verification (INSECURE=1 set)."
        red "         Install cosign: https://docs.sigstore.dev/cosign/installation/"
    elif [ -n "$INSECURE" ]; then
        red "WARNING: INSECURE=1 is set — cosign signature verification SKIPPED."
        red "         SHA256 verification still runs; unset INSECURE to re-enable cosign."
    fi

    # okdctl is Linux-only. Refuse to install on anything else. OS/ARCH must
    # match .goreleaser.yaml's name_template ({{ .Os }}_{{ .Arch }} — GOOS/GOARCH
    # spellings), which produces okdctl_<v>_linux_{amd64,arm64}.tar.gz.
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$OS" in
        linux) ;;
        *) die "unsupported OS: $OS — okdctl runs on Linux only (the deploy phase needs dnf/apt, systemd, firewall-cmd, nmcli)" ;;
    esac

    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64 | amd64) ARCH="amd64" ;;
        aarch64 | arm64) ARCH="arm64" ;;
        *) die "unsupported arch: $ARCH (supported: x86_64, arm64)" ;;
    esac

    # Resolve latest version if not pinned. Pattern adapted from get.helm.sh
    # (sed -n + capture group), more robust than grep | head | cut against
    # JSON key reordering or whitespace variation in the API response.
    if [ -z "$VERSION" ]; then
        info "resolving latest release..."
        # Bearer token travels via curl --config on stdin, not -H argv, so it
        # never shows up in ps / /proc/PID/cmdline on shared hosts. Lifts the
        # GitHub API rate limit from 60 to 5 000 req/hr per IP when set.
        _gh_config_line=""
        if [ -n "${GITHUB_TOKEN:-}" ]; then
            _gh_config_line=$(printf 'header = "Authorization: Bearer %s"' "$GITHUB_TOKEN")
        fi
        VERSION=$(printf '%s\n' "$_gh_config_line" |
            curl_safe -sSfL --max-time 30 --config - \
            "https://api.github.com/repos/$REPO/releases/latest" |
            sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' |
            head -1)
        [ -n "$VERSION" ] || die "failed to resolve latest release from GitHub API; pin VERSION=vX.Y.Z or set GITHUB_TOKEN to avoid rate-limiting"
        info "latest: $VERSION"
    fi

    [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$ ]] ||
        die "unexpected version tag: $VERSION"

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

    # sha256sum is now required, so SHA256SUMS is always consumed.
    info "downloading SHA256SUMS"
    curl_safe -sSfL -o "$TMP/SHA256SUMS" "$SHA_URL" ||
        die "failed to download SHA256SUMS from $SHA_URL"

    # Cosign verify-blob against the sigstore-published signature. Runs when
    # cosign is present and INSECURE is unset. INSECURE=1 is only reachable here
    # when cosign is installed (enforced above), so the shed layer is a user choice.
    if [ -n "$COSIGN_CMD" ] && [ -z "$INSECURE" ]; then
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
            --certificate-identity-regexp='^https://github\.com/qxtaiba/okdctl/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$' \
            --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
            "$TMP/SHA256SUMS" >/dev/null ||
            die "cosign signature verification failed on SHA256SUMS"
        info "cosign signature verified"
    elif [ -n "$INSECURE" ]; then
        info "cosign signature verification skipped (INSECURE=1)"
    fi

    # SHA256 verification is mandatory; sha256sum was required at startup.
    # awk field-equality (not grep) avoids treating '.' in the filename as a
    # regex wildcard.
    info "verifying SHA256"
    EXPECTED=$(awk -v name="$ARCHIVE_NAME" '$2 == name || $2 == "*"name {print $1}' "$TMP/SHA256SUMS")
    [ -n "$EXPECTED" ] || die "no checksum found for $ARCHIVE_NAME in SHA256SUMS"
    ACTUAL=$($SHA_CMD "$TMP/$ARCHIVE_NAME" | awk '{print $1}')
    [ "$EXPECTED" = "$ACTUAL" ] ||
        die "checksum mismatch: expected $EXPECTED, got $ACTUAL"
    info "checksum verified"

    # Extract the archive. --no-same-owner and --no-same-permissions harden
    # against a release tarball that encodes unexpected ownership. The cosign
    # + sha256 verification above is the primary guard; these are defense-in-depth.
    info "extracting"
    cd "$TMP"
    # Defense-in-depth: reject archives containing absolute paths or parent-traversal
    # entries before any bytes hit the filesystem. Goreleaser tarballs are flat, so
    # a match here means a tampered or malformed archive slipped past the sha256 check.
    tar -tzf "$ARCHIVE_NAME" | grep -qE '(^|/)\.\.(/|$)|^/' && die "archive contains absolute or parent-traversal paths"
    tar --no-same-owner --no-same-permissions --no-overwrite-dir -xzf "$ARCHIVE_NAME"

    [ -f "$BINARY" ] || die "$BINARY not found in archive"

    # Install. If the install dir is not writable by the current user, try sudo.
    # A nonexistent dir fails -w too and would silently route into the sudo
    # branch, dying with a raw coreutils error instead of this diagnostic.
    [ -d "$INSTALL_DIR" ] || die "INSTALL_DIR does not exist: $INSTALL_DIR"
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
}

# The wrapper makes bash parse the whole script before executing anything:
# a curl|bash transfer truncated mid-stream would otherwise run a prefix
# of the script (rustup / get.helm.sh installer pattern).
main "$@"
