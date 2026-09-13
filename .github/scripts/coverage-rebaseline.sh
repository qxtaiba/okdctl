#!/usr/bin/env bash
# Rewrites .github/coverage-floors.conf from a coverage profile, preserving the
# header. Floors are measured minus max(10, one statement's worth), rounded
# down to 5, so no floor can fail on a single uncovered statement.
# Generate the profile on LINUX: CI runs ubuntu and several packages measure
# differently on darwin, which silently skews every floor derived from them.
set -euo pipefail

FLOORS_FILE="$(dirname "$0")/../coverage-floors.conf"
COVERAGE_FILE="${1:-coverage.out}"

if [[ ! -f "$COVERAGE_FILE" ]]; then
  echo "coverage-rebaseline: $COVERAGE_FILE not found — run 'go test -coverprofile=$COVERAGE_FILE ./...'" >&2
  exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

# Header = every line up to and including the last comment before the entries.
awk '/^[[:space:]]*#/ || /^[[:space:]]*$/ { print; next } { exit }' "$FLOORS_FILE" > "$tmp"

awk '
FNR == 1 { next }
{
  pkg = $1
  sub(/:[^:]*$/, "", pkg)
  sub(/\/[^\/]*$/, "", pkg)
  stmts[pkg] += $2
  all_stmts += $2
  if ($3 > 0) { covered[pkg] += $2; all_covered += $2 }
}
function floor5(v) { return (v <= 0) ? 0 : int(v / 5) * 5 }
END {
  n = 0
  for (p in stmts) pkgs[++n] = p
  for (i = 1; i < n; i++)
    for (j = i + 1; j <= n; j++)
      if (pkgs[j] < pkgs[i]) { t = pkgs[i]; pkgs[i] = pkgs[j]; pkgs[j] = t }

  print "*=0"
  if (all_stmts > 0) printf "total=%d\n", floor5(100 * all_covered / all_stmts - 10)
  for (i = 1; i <= n; i++) {
    p = pkgs[i]
    pct = 100 * covered[p] / stmts[p]
    if (pct == 0) continue
    slack = 100 / stmts[p]
    if (slack < 10) slack = 10
    fl = floor5(pct - slack)
    if (fl > 0) printf "%s=%d\n", p, fl
  }
}
' "$COVERAGE_FILE" >> "$tmp"

mv "$tmp" "$FLOORS_FILE"
trap - EXIT
echo "coverage-rebaseline: rewrote $FLOORS_FILE from $COVERAGE_FILE"
