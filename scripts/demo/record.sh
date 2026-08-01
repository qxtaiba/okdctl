#!/usr/bin/env bash
# Regenerates docs/assets/demo.gif from docs/assets/demo.tape.
# Run from the repo root: scripts/demo/record.sh (or `make demo`).
#
# Requires: vhs (brew install vhs), go. The recording drives the real
# wizard against a local fake Proxmox API; nothing touches a hypervisor
# and nothing deploys (the tape quits from the review screen).
set -euo pipefail

command -v vhs >/dev/null || { echo "vhs not found — brew install vhs" >&2; exit 1; }

ROOT="$(git rev-parse --show-toplevel)"
WORK="$(mktemp -d -t okdctl-demo)"
# Guard the kill: an unset PVE_PID (die before fakepve starts) must not let
# errexit abort the handler before rm removes $WORK (holds a throwaway ed25519
# key + dummy pull secret). || true keeps the handler's exit status clean.
trap '[ -n "${PVE_PID:-}" ] && kill "$PVE_PID" 2>/dev/null || true; rm -rf "$WORK"' EXIT

# Demo home: dummy pull secret + throwaway ssh key so the files step
# validates without touching the operator's real credentials.
export OKDCTL_DEMO_HOME="$WORK/home"
mkdir -p "$OKDCTL_DEMO_HOME/.ssh"
echo '{"auths":{"fake":{"auth":"aWQ6cGFzcwo="}}}' > "$OKDCTL_DEMO_HOME/pull-secret.json"
ssh-keygen -q -t ed25519 -N '' -f "$OKDCTL_DEMO_HOME/.ssh/id_ed25519"

echo "building demo binary..."
go build -o "$WORK/okdctl" "$ROOT/cmd/okdctl"

echo "starting fake proxmox api..."
go run "$ROOT/scripts/demo/fakepve.go" &
PVE_PID=$!
for _ in $(seq 1 20); do
  curl -sk https://127.0.0.1:8006/api2/json/version >/dev/null 2>&1 && break
  sleep 0.5
done
curl -sk https://127.0.0.1:8006/api2/json/version >/dev/null 2>&1 ||
  { echo "fakepve did not become ready after 10s" >&2; exit 1; }

echo "recording (this replays the full tape in real time — ~4 minutes)..."
mkdir -p "$WORK/cwd"
OKDCTL_DEMO_BIN="$WORK/okdctl" OKDCTL_DEMO_CWD="$WORK/cwd" \
  vhs "$ROOT/docs/assets/demo.tape"

echo "done: docs/assets/demo.gif"
