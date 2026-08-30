#!/usr/bin/env bash
# okdctl installer: downloads a release from GitHub, verifies SHA256 (and
# cosign, when available) against SHA256SUMS, and installs to
# /usr/local/bin (or $INSTALL_DIR).
#
# Usage:
#   curl -sSfL https://raw.githubusercontent.com/qxtaiba/okdctl/develop/scripts/install.sh | bash
#
# Env vars:
#   VERSION       - pin a release, e.g. v0.1.0 (default: latest)
#   INSTALL_DIR   - install location (default: /usr/local/bin)
#   INSECURE      - "1" skips cosign verification (sha256-only trust)
#   GITHUB_TOKEN  - bearer token for the latest-release lookup; raises the
#                   GitHub API rate limit from 60 to 5000 req/hr/IP
#
# Requires: bash, curl, tar, sha256sum. Recommended: cosign.
#
# Trust layers: (1) TLS-only curl (transport-level MITM/downgrade
# rejection) (2) cosign verify-blob on SHA256SUMS, tied to the release
# workflow's OIDC identity (3) SHA256 of the archive against the
# cosign-verified sums (4) --no-same-permissions strips setuid/setgid
# bits on extract (5) install -m 0755 sets a known final mode.

set -euo pipefail

red()   { printf '\033[31m%s\033[0m\n' "$*" >&2; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
info()  { printf '  %s\n' "$*"; }
die()   { red "error: $*"; exit 1; }

# curl_safe: HTTPS-only (incl. redirects), TLS 1.2 floor, timeouts, 2 retries on connrefused.
curl_safe() { curl --proto '=https' --proto-redir '=https' --tlsv1.2 --connect-timeout 10 --max-time 120 --retry 2 --retry-connrefused "$@"; }

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

    # sha256sum ships with coreutils; refuse early rather than offering a bypass.
    command -v sha256sum >/dev/null 2>&1 ||
        die "sha256sum is required but not installed; install coreutils (e.g. apt-get install coreutils, dnf install coreutils)"

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

    # Linux-only; OS/ARCH must match .goreleaser.yaml's name_template
    # (produces okdctl_<v>_linux_{amd64,arm64}.tar.gz).
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

    # Resolve latest version if unpinned; sed -n + capture group (get.helm.sh
    # pattern) is robust to JSON key reordering/whitespace, unlike grep|head|cut.
    if [ -z "$VERSION" ]; then
        info "resolving latest release..."
        # Token travels via curl --config on stdin (not -H argv), so it
        # never appears in ps / /proc/PID/cmdline on shared hosts.
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

    TMP=$(mktemp -d 2>/dev/null || mktemp -d -t "$BINARY")
    # EXIT does cleanup; INT/TERM exit 130 (re-triggers EXIT) rather than
    # relying on set -e to trip later with a misleading status.
    trap 'rm -rf "$TMP"' EXIT
    trap 'exit 130' INT TERM

    info "downloading $ARCHIVE_NAME"
    curl_safe -sSfL -o "$TMP/$ARCHIVE_NAME" "$ARCHIVE_URL" ||
        die "failed to download $ARCHIVE_URL"

    info "downloading SHA256SUMS"
    curl_safe -sSfL -o "$TMP/SHA256SUMS" "$SHA_URL" ||
        die "failed to download SHA256SUMS from $SHA_URL"

    # Runs when cosign is present and INSECURE unset; INSECURE=1 here is a
    # deliberate user opt-out (cosign presence already enforced above).
    if [ -n "$COSIGN_CMD" ] && [ -z "$INSECURE" ]; then
        info "verifying cosign signature on SHA256SUMS"
        curl_safe -sSfL -o "$TMP/SHA256SUMS.sig" "$BASE_URL/SHA256SUMS.sig" ||
            die "failed to download SHA256SUMS.sig (release missing signature? uninstall cosign if you accept the risk)"
        curl_safe -sSfL -o "$TMP/SHA256SUMS.pem" "$BASE_URL/SHA256SUMS.pem" ||
            die "failed to download SHA256SUMS.pem"
        # stderr passes through so the user sees cosign's diagnostic (cert
        # identity, OIDC issuer, mismatch) instead of a bare failure.
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

    # awk field-equality (not grep) avoids treating '.' in the filename as a
    # regex wildcard.
    info "verifying SHA256"
    EXPECTED=$(awk -v name="$ARCHIVE_NAME" '$2 == name || $2 == "*"name {print $1}' "$TMP/SHA256SUMS")
    [ -n "$EXPECTED" ] || die "no checksum found for $ARCHIVE_NAME in SHA256SUMS"
    ACTUAL=$(sha256sum "$TMP/$ARCHIVE_NAME" | awk '{print $1}')
    [ "$EXPECTED" = "$ACTUAL" ] ||
        die "checksum mismatch: expected $EXPECTED, got $ACTUAL"
    info "checksum verified"

    # --no-same-owner/--no-same-permissions harden against unexpected tarball
    # ownership/modes; defense-in-depth behind the cosign+sha256 checks above.
    info "extracting"
    cd "$TMP"
    # Reject absolute/parent-traversal paths before any bytes hit disk (a
    # match means a tampered archive passed the sha256 check — goreleaser
    # tarballs are flat). Capture the listing first: piping tar | grep -q
    # would let grep exit early and SIGPIPE-kill tar under pipefail+set -e,
    # skipping die and failing open on a malicious archive.
    entries=$(tar -tzf "$ARCHIVE_NAME")
    if grep -qE '(^|/)\.\.(/|$)|^/' <<<"$entries"; then
        die "archive contains absolute or parent-traversal paths"
    fi
    tar --no-same-owner --no-same-permissions --no-overwrite-dir -xzf "$ARCHIVE_NAME"

    # -f follows symlinks, so a symlinked okdctl member would pass -f; the
    # traversal grep above checks names only, never link targets.
    [ ! -L "$BINARY" ] || die "$BINARY in archive is a symlink; refusing"
    [ -f "$BINARY" ] || die "$BINARY not found in archive"

    # A nonexistent INSTALL_DIR also fails -w, which would silently route into
    # the sudo branch and die with a raw coreutils error instead of this one.
    [ -d "$INSTALL_DIR" ] || die "INSTALL_DIR does not exist: $INSTALL_DIR"
    info "installing to $INSTALL_DIR/$BINARY"
    # Stage to a temp name, then atomically rename (mv -f, same filesystem):
    # an interrupt or ENOSPC mid-write leaves the temp file, never a
    # truncated okdctl on PATH.
    TMP_BIN="$INSTALL_DIR/.$BINARY.tmp.$$"
    if [ -w "$INSTALL_DIR" ]; then
        install -m 0755 "$BINARY" "$TMP_BIN" || die "stage $TMP_BIN"
        mv -f "$TMP_BIN" "$INSTALL_DIR/$BINARY" || { rm -f "$TMP_BIN"; die "install $INSTALL_DIR/$BINARY"; }
    elif command -v sudo >/dev/null 2>&1; then
        sudo install -m 0755 "$BINARY" "$TMP_BIN" || die "stage $TMP_BIN"
        sudo mv -f "$TMP_BIN" "$INSTALL_DIR/$BINARY" || { sudo rm -f "$TMP_BIN"; die "install $INSTALL_DIR/$BINARY"; }
    else
        die "$INSTALL_DIR is not writable and sudo is not available"
    fi

    green "okdctl $VERSION installed to $INSTALL_DIR/$BINARY"
    info "run 'okdctl doctor' to verify your environment"
}

# Wrapping in main() makes bash parse the whole script before running any
# of it, so a truncated curl|bash transfer can't execute a partial prefix.
main "$@"
