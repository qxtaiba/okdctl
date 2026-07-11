#!/usr/bin/env bash
# Enforces per-package statement-coverage floors defined in
# .github/coverage-floors.conf. Coverage is covered/total statements per
# package computed from the raw profile (mode: set) — not a per-function
# mean, which overweights trivial one-line functions.
# A floored package missing from the profile is a hard failure so the
# gate cannot be bypassed by deleting a package's tests.
# Logic lives in POSIX awk so the script runs under macOS bash 3.2
# (no associative arrays) as well as CI's bash 5 + mawk.
# Runs after: go test -coverprofile=coverage.out ./...
set -euo pipefail

FLOORS_FILE="$(dirname "$0")/../coverage-floors.conf"
COVERAGE_FILE="${1:-coverage.out}"

if [[ ! -f "$COVERAGE_FILE" ]]; then
  echo "coverage-check: $COVERAGE_FILE not found — run 'go test -coverprofile=$COVERAGE_FILE ./...'" >&2
  exit 1
fi

if [[ ! -f "$FLOORS_FILE" ]]; then
  echo "coverage-check: $FLOORS_FILE not found" >&2
  exit 1
fi

awk '
BEGIN { def = 0; total_floor = 0 }

# First file: floors (pkg=pct, "#" comments, "*" default, "total" aggregate).
FNR == NR {
  if ($0 ~ /^[[:space:]]*#/) next
  line = $0
  gsub(/[[:space:]]/, "", line)
  if (line == "") next
  eq = index(line, "=")
  if (eq == 0) next
  key = substr(line, 1, eq - 1)
  val = substr(line, eq + 1) + 0
  if (key == "*") def = val
  else if (key == "total") total_floor = val
  else floor[key] = val
  next
}

# Second file: coverage profile. Skip the "mode:" header; remaining lines
# are "<file>:<range> <numStmts> <hitCount>". A block is covered when its
# hit count is non-zero.
FNR == 1 { next }
{
  pkg = $1
  sub(/:[^:]*$/, "", pkg)
  sub(/\/[^\/]*$/, "", pkg)
  stmts[pkg] += $2
  all_stmts += $2
  if ($3 > 0) {
    covered[pkg] += $2
    all_covered += $2
  }
}

function report(pkg, pct, fl) {
  if (pct < fl) {
    printf "FAIL  %-70s  %5.1f%% (floor %d%%)\n", pkg, pct, fl > "/dev/stderr"
    return 1
  }
  printf "ok    %-70s  %5.1f%% (floor %d%%)\n", pkg, pct, fl
  return 0
}

END {
  n = 0
  for (p in stmts) pkgs[++n] = p
  for (i = 1; i < n; i++)
    for (j = i + 1; j <= n; j++)
      if (pkgs[j] < pkgs[i]) { t = pkgs[i]; pkgs[i] = pkgs[j]; pkgs[j] = t }

  failed = 0
  for (i = 1; i <= n; i++) {
    p = pkgs[i]
    fl = (p in floor) ? floor[p] : def
    if (report(p, 100 * covered[p] / stmts[p], fl)) failed = 1
  }

  for (p in floor)
    if (!(p in stmts)) {
      printf "FAIL  %-70s  absent from coverage profile (floor %d%%) - were its tests deleted?\n", \
        p, floor[p] > "/dev/stderr"
      failed = 1
    }

  if (all_stmts > 0) {
    if (report("total", 100 * all_covered / all_stmts, total_floor)) failed = 1
  } else {
    print "coverage-check: no coverage data in profile" > "/dev/stderr"
    failed = 1
  }

  if (failed) {
    print "" > "/dev/stderr"
    print "coverage-check: one or more packages are below their floor." > "/dev/stderr"
    print "Raise test coverage or update .github/coverage-floors.conf." > "/dev/stderr"
    exit 1
  }
}
' "$FLOORS_FILE" "$COVERAGE_FILE"
