#!/usr/bin/env bash
# Enforces per-package coverage floors defined in .github/coverage-floors.conf.
# Exit non-zero if any package falls below its floor. Runs after
# go test -coverprofile=coverage.out ./... has already produced coverage.out.
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

declare -A floors
default_floor=0
while IFS='=' read -r pkg pct; do
  [[ "$pkg" =~ ^#.*$ || -z "$pkg" ]] && continue
  pkg="${pkg// /}"
  pct="${pct// /}"
  if [[ "$pkg" == "*" ]]; then
    default_floor="$pct"
  else
    floors["$pkg"]="$pct"
  fi
done < "$FLOORS_FILE"

declare -A pkg_total
declare -A pkg_count
total_pct=""

while IFS= read -r line; do
  if [[ "$line" =~ ^total: ]]; then
    total_pct=$(echo "$line" | awk '{gsub(/%/,"",$NF); print $NF}')
    continue
  fi
  file_path=$(echo "$line" | awk '{print $1}' | cut -d: -f1)
  pct_raw=$(echo "$line" | awk '{gsub(/%/,"",$NF); print $NF}')
  pkg_path="${file_path%/*}"
  pkg_total["$pkg_path"]=$(echo "${pkg_total[$pkg_path]:-0} + $pct_raw" | bc)
  pkg_count["$pkg_path"]=$(( ${pkg_count[$pkg_path]:-0} + 1 ))
done < <(go tool cover -func="$COVERAGE_FILE" 2>/dev/null)

failed=0

for pkg in "${!pkg_total[@]}"; do
  count="${pkg_count[$pkg]}"
  if [[ "$count" -eq 0 ]]; then continue; fi
  avg=$(echo "scale=1; ${pkg_total[$pkg]} / $count" | bc)
  floor="${floors[$pkg]:-$default_floor}"
  result=$(echo "$avg < $floor" | bc)
  if [[ "$result" -eq 1 ]]; then
    printf "FAIL  %-70s  %.1f%% (floor %s%%)\n" "$pkg" "$avg" "$floor" >&2
    failed=1
  else
    printf "ok    %-70s  %.1f%% (floor %s%%)\n" "$pkg" "$avg" "$floor"
  fi
done

if [[ -n "$total_pct" ]]; then
  total_floor="${floors[total]:-$default_floor}"
  result=$(echo "$total_pct < $total_floor" | bc)
  if [[ "$result" -eq 1 ]]; then
    printf "FAIL  %-70s  %s%% (floor %s%%)\n" "total" "$total_pct" "$total_floor" >&2
    failed=1
  else
    printf "ok    %-70s  %s%% (floor %s%%)\n" "total" "$total_pct" "$total_floor"
  fi
fi

if [[ "$failed" -eq 1 ]]; then
  echo "" >&2
  echo "coverage-check: one or more packages are below their floor." >&2
  echo "Raise test coverage or update .github/coverage-floors.conf." >&2
  exit 1
fi
