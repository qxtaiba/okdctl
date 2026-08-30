#!/usr/bin/env bash
# Refreshes the streamPins map in coreos.go from openshift/installer
# release-4.X branch tips. Run via .github/workflows/update-coreos-pins.yml
# on a schedule, or manually before a release.
#
# streamPins is keyed by (major, minor); MAJOR=4 until OKD 5.x exists, at
# which point add a second major/minor loop rather than changing MAJOR.
#
# Per minor: resolve branch tip -> commit SHA, fetch fcos.json (<=4.18) or
# scos.json (4.19+) at that commit, sha256 it, and rewrite streamPins.
# Exits 0 with a clean tree when no drift was found.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COREOS_GO="$ROOT/internal/distribution/okd/provision/coreos.go"
MAJOR=4
SUPPORTED_MINORS=(10 11 12 13 14 15 16 17 18 19 20 21 22 23)

require() { command -v "$1" >/dev/null 2>&1 || { echo "missing: $1" >&2; exit 1; }; }
require curl
require sha256sum
require git
require python3

env_args=()
for minor in "${SUPPORTED_MINORS[@]}"; do
    flavor=fcos
    # fcos/scos split at 19 is specific to MAJOR=4; a future major loop must
    # define its own boundary, not reuse 19.
    [ "$minor" -ge 19 ] && flavor=scos
    # --heads + anchored refs/heads/ prevents a like-named tag from matching;
    # NR==1 takes one sha even if the pattern ever widens.
    sha=$(git ls-remote --heads https://github.com/openshift/installer "refs/heads/release-$MAJOR.$minor" | awk 'NR==1{print $1}')
    if [ -z "$sha" ]; then
        echo "warn: no tip for release-$MAJOR.$minor — skipping" >&2
        continue
    fi
    # Reject empty/non-JSON before hashing: a truncated 200 would otherwise
    # hash to a plausible pin (empty-input sha256 e3b0c442...) and make a
    # degenerate stream verify.
    json_body=$(curl -sSfL --proto '=https' --proto-redir '=https' --tlsv1.2 --connect-timeout 10 --max-time 60 "https://raw.githubusercontent.com/openshift/installer/${sha}/data/data/coreos/${flavor}.json")
    if [ -z "$json_body" ] || ! printf '%s' "$json_body" | python3 -c 'import json,sys; json.load(sys.stdin)' 2>/dev/null; then
        echo "warn: empty or non-JSON ${flavor}.json for release-$MAJOR.$minor — skipping" >&2
        continue
    fi
    json_sha=$(printf '%s' "$json_body" | sha256sum | awk '{print $1}')
    env_args+=("PIN_SHA_${minor}=${sha}" "PIN_JSON_${minor}=${json_sha}")
done

env "${env_args[@]}" python3 - "$COREOS_GO" "$MAJOR" "${SUPPORTED_MINORS[@]}" <<'PY'
import os
import re
import sys
from pathlib import Path

path = Path(sys.argv[1])
major = int(sys.argv[2])
minors = [int(m) for m in sys.argv[3:]]
text = path.read_text()

block_re = re.compile(
    r"^var streamPins = map\[okdVersionKey\]coreOSStreamPin\{$.*?^\}$",
    re.DOTALL | re.MULTILINE,
)
block = block_re.search(text)
if block is None:
    raise SystemExit("streamPins block not found in coreos.go")

# Merge onto the committed map rather than rebuilding: a minor that failed
# to resolve this run keeps its old pin instead of being dropped, and a
# minor hand-added but absent from SUPPORTED_MINORS survives too.
entry_re = re.compile(
    r'\{(\d+),\s*(\d+)\}:\s*\{CommitSHA:\s*"([0-9a-f]+)",\s*JSONSHA256:\s*"([0-9a-f]+)"\}'
)
pin_map = {
    (int(maj), int(minr)): (sha, json_sha)
    for maj, minr, sha, json_sha in entry_re.findall(block.group(0))
}

resolved = 0
for minor in minors:
    sha = os.environ.get(f"PIN_SHA_{minor}", "")
    json_sha = os.environ.get(f"PIN_JSON_{minor}", "")
    if sha and json_sha:
        pin_map[(major, minor)] = (sha, json_sha)
        resolved += 1

if resolved == 0:
    raise SystemExit("no pins resolved — check network access to github.com")

lines = []
for maj, minr in sorted(pin_map):
    sha, json_sha = pin_map[(maj, minr)]
    lines.append(f'\t{{{maj}, {minr}}}: {{CommitSHA: "{sha}", JSONSHA256: "{json_sha}"}},')
new_block = "var streamPins = map[okdVersionKey]coreOSStreamPin{\n" + "\n".join(lines) + "\n}"

new_text = text[: block.start()] + new_block + text[block.end() :]

if new_text == text:
    print("coreos pins: no drift")
else:
    path.write_text(new_text)
    print("coreos pins: updated")
PY
