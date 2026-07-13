#!/usr/bin/env bash
# update-coreos-pins.sh refreshes the streamPins map in coreos.go from
# upstream openshift/installer release-4.X branch tips. Run via
# .github/workflows/update-coreos-pins.yml on a schedule, or manually
# before a release to land any drift in a reviewable PR.
#
# streamPins is keyed by (major, minor); MAJOR is fixed at 4 here because
# every currently supported minor is a 4.x release. Bumping onto a new major
# (e.g. once OKD 5.x is generally available) means adding a second
# major/minor loop rather than changing MAJOR in place.
#
# For each supported OKD minor:
#   1. resolve release-4.X tip → commit SHA
#   2. fetch fcos.json (4.15-4.18) or scos.json (4.19+) at that commit
#   3. compute sha256 of the JSON body
#   4. rewrite the streamPins map in coreos.go
#
# Exits 0 with a clean working tree when no drift was found.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COREOS_GO="$ROOT/internal/distribution/okd/setup/coreos.go"
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
    [ "$minor" -ge 19 ] && flavor=scos
    sha=$(git ls-remote https://github.com/openshift/installer "release-$MAJOR.$minor" | awk '{print $1}')
    if [ -z "$sha" ]; then
        echo "warn: no tip for release-$MAJOR.$minor — skipping" >&2
        continue
    fi
    json_sha=$(curl -sSfL --proto '=https' --tlsv1.2 --connect-timeout 10 --max-time 60 "https://raw.githubusercontent.com/openshift/installer/${sha}/data/data/coreos/${flavor}.json" | sha256sum | awk '{print $1}')
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

pin_map = {}
for minor in minors:
    sha = os.environ.get(f"PIN_SHA_{minor}", "")
    json_sha = os.environ.get(f"PIN_JSON_{minor}", "")
    if sha and json_sha:
        pin_map[minor] = (sha, json_sha)

if not pin_map:
    raise SystemExit("no pins resolved — check network access to github.com")

lines = []
for minor in sorted(pin_map):
    sha, json_sha = pin_map[minor]
    lines.append(f'\t{{{major}, {minor}}}: {{CommitSHA: "{sha}", JSONSHA256: "{json_sha}"}},')
new_block = "var streamPins = map[okdVersionKey]coreOSStreamPin{\n" + "\n".join(lines) + "\n}"

new_text, n = re.subn(
    r"^var streamPins = map\[okdVersionKey\]coreOSStreamPin\{$.*?^\}$",
    new_block,
    text,
    count=1,
    flags=re.DOTALL | re.MULTILINE,
)
if n != 1:
    raise SystemExit("streamPins block not found in coreos.go")

if new_text == text:
    print("coreos pins: no drift")
else:
    path.write_text(new_text)
    print("coreos pins: updated")
PY
