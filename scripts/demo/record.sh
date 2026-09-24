#!/usr/bin/env bash
# Regenerates docs/assets/demo.gif from docs/assets/demo.tape (or `make demo`).
# Requires vhs + go; drives the real wizard against a fake Proxmox API — nothing
# touches a hypervisor or deploys.
set -euo pipefail

command -v vhs >/dev/null || { echo "vhs not found — brew install vhs" >&2; exit 1; }

ROOT="$(git rev-parse --show-toplevel)"
WORK="$(mktemp -d -t okdctl-demo)"
# Guards the kill: unset PVE_PID must not abort before rm cleans $WORK under
# errexit; || true keeps the handler's exit clean.
trap '[ -n "${PVE_PID:-}" ] && kill "$PVE_PID" 2>/dev/null || true; rm -rf "$WORK"' EXIT

export OKDCTL_DEMO_HOME="$WORK/home"
mkdir -p "$OKDCTL_DEMO_HOME"

# Dummy pull secret + throwaway ssh key, placed in the demo's cwd (not HOME):
# ExpandPath's "~" resolves via the invoking OS user's real home regardless
# of $HOME (see internal/system/elevation.go, InvokingUserHomeDir), so a
# HOME override can't steer it — the files step must reach these by a
# cwd-relative path instead. This keeps the files step validating without
# touching real credentials.
mkdir -p "$WORK/cwd/.ssh"
echo '{"auths":{"fake":{"auth":"aWQ6cGFzcwo="}}}' > "$WORK/cwd/pull-secret.json"
ssh-keygen -q -t ed25519 -N '' -f "$WORK/cwd/.ssh/id_ed25519"

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
OKDCTL_DEMO_BIN="$WORK/okdctl" OKDCTL_DEMO_CWD="$WORK/cwd" \
  vhs "$ROOT/docs/assets/demo.tape"

echo "done: docs/assets/demo.gif"
