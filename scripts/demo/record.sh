#!/usr/bin/env bash
# Regenerates docs/assets/demo.gif from docs/assets/demo.tape (or `make demo`).
# Requires vhs + go; drives the real wizard against a fake Proxmox API and
# OKDCTL_WIZARD_DEMO's scripted deploy feed — nothing touches a hypervisor,
# and the deploy stream's finish screen is the scripted feed's outcome, not
# a real one.
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

# The hero-hub shows its five-verb menu (deploy/edit config/manage nodes/
# cluster status/destroy) and the tape's opening "cluster status" gate only
# when a saved okdctl.yaml is present — reusing scripts/screenshot's fixture
# keeps this in lockstep with the wizard tape's own hub keystrokes. Checked
# up front, the same way run.sh does: an unloadable fixture silently drops
# the hub back to its two-verb blank slate, which the tape's keystrokes
# would then walk straight into "quit".
DEMO_CONFIG="$ROOT/scripts/screenshot/demo-config.yaml"
DEMO_CLUSTER=homelab
if ! "$WORK/okdctl" config validate --config "$DEMO_CONFIG" >/dev/null ||
   ! "$WORK/okdctl" config show --config "$DEMO_CONFIG" | grep -q "name: $DEMO_CLUSTER"; then
  echo "scripts/screenshot/demo-config.yaml does not load as cluster '$DEMO_CLUSTER' — fix it before recording" >&2
  exit 1
fi
cp "$DEMO_CONFIG" "$WORK/cwd/okdctl.yaml"

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
