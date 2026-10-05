#!/usr/bin/env bash
# Renders the 11-step configure wizard against the fakepve fixture, plus the
# Cluster Lifecycle wizard against lifecycle.DemoHooks' static fixture, at
# four terminal sizes (80x24, 100x30, 120x40, 180x48), screenshotting every
# step. Requires vhs (github.com/charmbracelet/vhs) + go; nothing touches a
# real hypervisor or deploys.
set -euo pipefail

command -v vhs >/dev/null || { echo "vhs not found — brew install vhs" >&2; exit 1; }
command -v python3 >/dev/null || { echo "python3 not found — brew install python3" >&2; exit 1; }

ROOT="$(git rev-parse --show-toplevel)"
SCREENSHOT_DIR="$ROOT/scripts/screenshot"
OUT_DIR="$SCREENSHOT_DIR/out"
WORK="$(mktemp -d -t okdctl-screenshot)"
# The trap no-ops until PVE_PID is set, so early failures cannot kill a
# process this runner did not start.
trap '[ -n "${PVE_PID:-}" ] && kill "$PVE_PID" 2>/dev/null || true; rm -rf "$WORK"' EXIT

PVE_PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"

mkdir -p "$OUT_DIR"

# Dummy pull secret + throwaway ssh key so the files step validates without
# touching real credentials.
export OKDCTL_DEMO_HOME="$WORK/home"
mkdir -p "$OKDCTL_DEMO_HOME/.ssh"
echo '{"auths":{"fake":{"auth":"aWQ6cGFzcwo="}}}' > "$OKDCTL_DEMO_HOME/pull-secret.json"
ssh-keygen -q -t ed25519 -N '' -f "$OKDCTL_DEMO_HOME/.ssh/id_ed25519"

echo "building demo binary..."
export OKDCTL_DEMO_BIN="$WORK/okdctl"
go build -o "$OKDCTL_DEMO_BIN" "$ROOT/cmd/okdctl"

# The hero-hub shows its five-verb menu and save-slot line only when a saved
# okdctl.yaml is present, so every render cwd gets this fixture (seed_cwd).
# Checked up front: an unloadable fixture silently drops the hub back to its
# two-verb blank slate, which the tapes' keystrokes would then walk into
# "quit". `config validate` alone is not enough — a fixture whose body the
# loader skips still validates, since compiled-in defaults are themselves
# valid — so the check reads the loaded cluster name back.
DEMO_CONFIG="$SCREENSHOT_DIR/demo-config.yaml"
DEMO_CLUSTER=homelab
if ! "$OKDCTL_DEMO_BIN" config validate --config "$DEMO_CONFIG" >/dev/null ||
   ! "$OKDCTL_DEMO_BIN" config show --config "$DEMO_CONFIG" | grep -q "name: $DEMO_CLUSTER"; then
  echo "scripts/screenshot/demo-config.yaml does not load as cluster '$DEMO_CLUSTER' — fix it before rendering" >&2
  exit 1
fi

# seed_cwd creates a render cwd holding the demo configuration.
seed_cwd() {
  mkdir -p "$1"
  sed "s/127.0.0.1:8006/127.0.0.1:$PVE_PORT/" "$DEMO_CONFIG" > "$1/okdctl.yaml"
  mkdir -p "$1/d"
  cp "$OKDCTL_DEMO_HOME/pull-secret.json" "$1/d/p"
  cp "$OKDCTL_DEMO_HOME/.ssh/id_ed25519.pub" "$1/d/k"
}

# seed_deployed_cwd adds terraform state holding a resource, which is what
# makes the hub's save-slot line read "deployed" rather than "configured".
seed_deployed_cwd() {
  seed_cwd "$1"
  local state="$1/infrastructure/terraform/environments/production/terraform.tfstate"
  mkdir -p "$(dirname "$state")"
  cat > "$state" <<'EOF'
{"version":4,"resources":[{"mode":"managed","type":"proxmox_virtual_environment_vm","name":"demo","instances":[{}]}]}
EOF
}

echo "starting fake proxmox api..."
# The repository fixture binds :8006; compile a temporary port-substituted
# copy so screenshots never contact or stop a user's local Proxmox service.
sed "s/127.0.0.1:8006/127.0.0.1:$PVE_PORT/g" "$ROOT/scripts/demo/fakepve.go" > "$WORK/fakepve.go"
go build -o "$WORK/fakepve" "$WORK/fakepve.go"
"$WORK/fakepve" > "$WORK/fakepve.log" 2>&1 &
PVE_PID=$!
for _ in $(seq 1 20); do
  curl -sk "https://127.0.0.1:$PVE_PORT/api2/json/version" >/dev/null 2>&1 && break
  sleep 0.5
done
curl -sk "https://127.0.0.1:$PVE_PORT/api2/json/version" >/dev/null 2>&1 ||
  { cat "$WORK/fakepve.log" >&2; echo "fakepve did not become ready on 127.0.0.1:$PVE_PORT after 10s" >&2; exit 1; }

# name:cols:rows:width:height. Width/height are pre-calibrated on this
# machine (vhs 0.11.0, FontSize 14) starting from W=cols*8.4, H=rows*17 and
# nudged until the calibration tape prints exactly "cols rows"; the runtime
# check below re-verifies this on every run and aborts with the measured
# numbers if font rendering drifts on another machine.
PRESETS=(
  "80x24:80:24:750:408"
  "100x30:100:30:930:492"
  "120x40:120:40:1108:652"
  "180x48:180:48:1650:784"
)
if [ -n "${SCREENSHOT_ONLY:-}" ]; then
  filtered=()
  for preset in "${PRESETS[@]}"; do
    [[ "$preset" == "$SCREENSHOT_ONLY:"* ]] && filtered+=("$preset")
  done
  [ "${#filtered[@]}" -gt 0 ] || { echo "unknown SCREENSHOT_ONLY preset: $SCREENSHOT_ONLY" >&2; exit 2; }
  PRESETS=("${filtered[@]}")
fi

# Screens in wizard order; must match the Screenshot
# filenames baked into wizard.tape.in.
STEP_NAMES=(welcome distribution proxmox basics node-placement networking resources addons files advanced review)

# Screens in lifecycle.NewSteps order; must match the Screenshot filenames
# baked into lifecycle.tape.in.
LIFECYCLE_STEP_NAMES=(op target params preview confirm exec "done")

# Screens in hub.tape.in's capture order; must match the Screenshot
# filenames baked into that tape. Kept as the single source of truth so the
# cleanup and the missing-screen check can never drift apart.
HUB_STEP_NAMES=(hub-deployed cluster-status cluster-status-details hub-manage)

render_tape() {
  local template="$1" name="$2" w="$3" h="$4" dest="$5"
  sed -e "s/@W@/$w/g; s/@H@/$h/g; s/@NAME@/$name/g; s/@PVEPORT@/$PVE_PORT/g" -e "s|@HOME@|$OKDCTL_DEMO_HOME|g" "$template" > "$dest"
}

calibrate() {
  local name="$1" cols="$2" rows="$3" w="$4" h="$5"
  local tape="$WORK/calib-$name.tape"
  render_tape "$SCREENSHOT_DIR/calibrate.tape.in" "$name" "$w" "$h" "$tape"
  (cd "$SCREENSHOT_DIR" && vhs "$tape" >/dev/null 2>&1)
  local measured
  measured="$(grep -E '^[0-9]+ [0-9]+$' "$OUT_DIR/calib-$name.txt" 2>/dev/null | tail -1 | tr -d '\r')"
  if [ "$measured" != "$cols $rows" ]; then
    echo "calibration mismatch for $name: wanted '$cols $rows', measured '$measured' (W=$w H=$h)" >&2
    echo "adjust the width/height preset in scripts/screenshot/run.sh and re-run" >&2
    exit 1
  fi
  echo "calibrated $name: W=$w H=$h -> $measured"
}

# vhs's Screenshot command occasionally fails its internal ffmpeg frame
# composite without any error (charmbracelet/vhs — no upstream issue filed
# yet); rendering succeeds deterministically once a good preset is found, but
# a fresh render is retried a few times as a safety net against transient
# misses.
render_wizard() {
  local name="$1" w="$2" h="$3"
  local attempt
  for step in "${STEP_NAMES[@]}"; do
    rm -f "$OUT_DIR/$name-$step.png"
  done
  rm -f "$OUT_DIR/$name-distribution-expanded.png"
  for attempt in 1 2 3; do
    local cwd="$WORK/cwd-$name-$attempt"
    seed_cwd "$cwd"
    local tape="$WORK/wizard-$name.tape"
    render_tape "$SCREENSHOT_DIR/wizard.tape.in" "$name" "$w" "$h" "$tape"

    echo "rendering $name (attempt $attempt)..."
    local log="$WORK/vhs-$name-wizard.log"
    (cd "$SCREENSHOT_DIR" && OKDCTL_DEMO_CWD="$cwd" vhs "$tape" >"$log" 2>&1) || true

    local missing=()
    local step
    for step in "${STEP_NAMES[@]}"; do
      local png="$OUT_DIR/$name-$step.png"
      [ -s "$png" ] || missing+=("$step")
    done

    if [ "${#missing[@]}" -eq 0 ]; then
      echo "rendered $name: ${#STEP_NAMES[@]}/${#STEP_NAMES[@]} screenshots"
      return 0
    fi
    echo "  missing after attempt $attempt: ${missing[*]}"
    cat "$log" >&2
  done

  echo "failed to render all screenshots for $name after 3 attempts; missing: ${missing[*]}" >&2
  return 1
}

# render_lifecycle renders the Cluster Lifecycle wizard ("okdctl node
# manage") against lifecycle.DemoHooks' static fixture, independently of the
# configure-wizard walkthrough above; same retry shape as render_wizard.
render_lifecycle() {
  local name="$1" w="$2" h="$3"
  local attempt
  for step in "${LIFECYCLE_STEP_NAMES[@]}"; do
    rm -f "$OUT_DIR/$name-lifecycle-$step.png"
  done
  for attempt in 1 2 3; do
    local cwd="$WORK/cwd-lifecycle-$name-$attempt"
    mkdir -p "$cwd"
    local tape="$WORK/lifecycle-$name.tape"
    render_tape "$SCREENSHOT_DIR/lifecycle.tape.in" "$name" "$w" "$h" "$tape"

    echo "rendering $name lifecycle (attempt $attempt)..."
    local log="$WORK/vhs-$name-lifecycle.log"
    (cd "$SCREENSHOT_DIR" && OKDCTL_DEMO_CWD="$cwd" vhs "$tape" >"$log" 2>&1) || true

    local missing=()
    local step
    for step in "${LIFECYCLE_STEP_NAMES[@]}"; do
      local png="$OUT_DIR/$name-lifecycle-$step.png"
      [ -s "$png" ] || missing+=("$step")
    done

    if [ "${#missing[@]}" -eq 0 ]; then
      echo "rendered $name: lifecycle ${#LIFECYCLE_STEP_NAMES[@]}/${#LIFECYCLE_STEP_NAMES[@]} screenshots"
      return 0
    fi
    echo "  missing after attempt $attempt: ${missing[*]}"
    cat "$log" >&2
  done

  echo "failed to render all lifecycle screenshots for $name after 3 attempts; missing: ${missing[*]}" >&2
  return 1
}

# render_distribution_fail renders just the distribution step under
# OKDCTL_DEMO_RELEASES=fail, pinning the error-state fixture (empty state +
# retry ribbon) independently of the full 11-step walkthrough above.
render_distribution_fail() {
  local name="$1" w="$2" h="$3"
  local attempt
  rm -f "$OUT_DIR/$name-distribution-fail.png"
  for attempt in 1 2 3; do
    local cwd="$WORK/cwd-fail-$name-$attempt"
    seed_cwd "$cwd"
    local tape="$WORK/distribution-fail-$name.tape"
    render_tape "$SCREENSHOT_DIR/distribution-fail.tape.in" "$name" "$w" "$h" "$tape"

    echo "rendering $name distribution-fail (attempt $attempt)..."
    local log="$WORK/vhs-$name-distribution-fail.log"
    (cd "$SCREENSHOT_DIR" && OKDCTL_DEMO_CWD="$cwd" vhs "$tape" >"$log" 2>&1) || true

    local png="$OUT_DIR/$name-distribution-fail.png"
    if [ -s "$png" ]; then
      echo "rendered $name: distribution-fail screenshot"
      return 0
    fi
    echo "  missing after attempt $attempt: distribution-fail"
    cat "$log" >&2
  done

  echo "failed to render the distribution-fail screenshot for $name after 3 attempts" >&2
  return 1
}

render_hub() {
  local name="$1" w="$2" h="$3"
  local attempt
  local screen
  for screen in "${HUB_STEP_NAMES[@]}"; do
    rm -f "$OUT_DIR/$name-$screen.png"
  done
  for attempt in 1 2 3; do
    local cwd="$WORK/cwd-hub-$name-$attempt"
    seed_deployed_cwd "$cwd"
    local tape="$WORK/hub-$name.tape"
    render_tape "$SCREENSHOT_DIR/hub.tape.in" "$name" "$w" "$h" "$tape"

    echo "rendering $name hub (attempt $attempt)..."
    local log="$WORK/vhs-$name-hub.log"
    (cd "$SCREENSHOT_DIR" && OKDCTL_DEMO_CWD="$cwd" vhs "$tape" >"$log" 2>&1) || true

    local missing=()
    for screen in "${HUB_STEP_NAMES[@]}"; do
      [ -s "$OUT_DIR/$name-$screen.png" ] || missing+=("$screen")
    done
    if [ "${#missing[@]}" -eq 0 ]; then
      echo "rendered $name hub: ${#HUB_STEP_NAMES[@]}/${#HUB_STEP_NAMES[@]} screenshots"
      return 0
    fi
    echo "  missing after attempt $attempt: ${missing[*]}"
    cat "$log" >&2
  done

  echo "failed to render the hub for $name after 3 attempts; missing: ${missing[*]}" >&2
  return 1
}

for preset in "${PRESETS[@]}"; do
  IFS=':' read -r name cols rows w h <<< "$preset"
  calibrate "$name" "$cols" "$rows" "$w" "$h"
  render_wizard "$name" "$w" "$h"
  render_hub "$name" "$w" "$h"
  render_distribution_fail "$name" "$w" "$h"
  render_lifecycle "$name" "$w" "$h"
done

echo "done: $OUT_DIR ($(find "$OUT_DIR" -name '*.png' | wc -l | tr -d ' ') PNGs)"
