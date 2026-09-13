#!/usr/bin/env bash
# Coverage = covered/total statements per package, not per-function mean; a
# floored package missing from the profile fails. POSIX awk only (bash 3.2, mawk).
# Run after: go test -coverprofile=coverage.out ./...
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
BEGIN { def = 0; total_floor = 0; stale = 0; stale_slack = 20 }

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

# Second file: coverage profile lines "<file>:<range> numStmts hitCount"; hitCount>0 = covered.
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

# STALE is advisory, never fatal: a floor this far below actual has stopped
# being a tripwire, and nothing else reports that it has gone slack.
function report(pkg, pct, fl,   slack) {
  if (pct < fl) {
    printf "FAIL  %-70s  %5.1f%% (floor %d%%)\n", pkg, pct, fl > "/dev/stderr"
    return 1
  }
  slack = pct - fl
  if (slack > stale_slack) {
    printf "STALE %-70s  %5.1f%% (floor %d%%, +%.1f) re-baseline\n", pkg, pct, fl, slack
    stale_names[++stale] = pkg
    return 0
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
      printf "FAIL  %-70s  absent from coverage profile (floor %d%%) - package deleted or renamed?\n", \
        p, floor[p] > "/dev/stderr"
      failed = 1
    }

  if (all_stmts > 0) {
    if (report("total", 100 * all_covered / all_stmts, total_floor)) failed = 1
  } else {
    print "coverage-check: no coverage data in profile" > "/dev/stderr"
    failed = 1
  }

  if (stale > 0) {
    names = stale_names[1]
    for (i = 2; i <= stale; i++) names = names ", " stale_names[i]
    printf "\ncoverage-check: %d entr%s drifted >%d points above %s floor; run: make coverage-floors.\n", \
      stale, (stale == 1 ? "y" : "ies"), stale_slack, (stale == 1 ? "its" : "their")
    # A green CI step has its log collapsed; the annotation is the only
    # form of this warning a reader actually sees.
    if (ENVIRON["GITHUB_ACTIONS"] == "true")
      printf "::warning title=Stale coverage floors::%d entr%s drifted >%d points above %s floor (%s); run: make coverage-floors\n", \
        stale, (stale == 1 ? "y" : "ies"), stale_slack, (stale == 1 ? "its" : "their"), names
  }

  if (failed) {
    print "" > "/dev/stderr"
    print "coverage-check: one or more packages are below their floor." > "/dev/stderr"
    print "Raise test coverage or update .github/coverage-floors.conf." > "/dev/stderr"
    exit 1
  }
}
' "$FLOORS_FILE" "$COVERAGE_FILE"
