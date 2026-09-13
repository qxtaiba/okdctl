#!/usr/bin/env bash
# Renders the 11-step configure wizard against the fakepve fixture at three
# terminal sizes (80x24, 100x30, 120x40), screenshotting the header of every
# step. Requires vhs (github.com/charmbracelet/vhs) + go; nothing touches a
# real hypervisor or deploys.
set -euo pipefail

command -v vhs >/dev/null || { echo "vhs not found — brew install vhs" >&2; exit 1; }

ROOT="$(git rev-parse --show-toplevel)"
SCREENSHOT_DIR="$ROOT/scripts/screenshot"
OUT_DIR="$SCREENSHOT_DIR/out"
WORK="$(mktemp -d -t okdctl-screenshot)"
# `go run` doesn't forward signals to the compiled child it spawns, so
# killing $PVE_PID (the go run wrapper) leaves fakepve itself running; kill
# by port instead. || true keeps the handler's exit clean under errexit.
trap '(lsof -ti :8006 | xargs kill) 2>/dev/null || true; rm -rf "$WORK"' EXIT

if lsof -i :8006 >/dev/null 2>&1; then
  echo "port 8006 is already in use — stop whatever's listening and retry" >&2
  exit 1
fi

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

echo "starting fake proxmox api..."
go run "$ROOT/scripts/demo/fakepve.go" &
for _ in $(seq 1 20); do
  curl -sk https://127.0.0.1:8006/api2/json/version >/dev/null 2>&1 && break
  sleep 0.5
done
curl -sk https://127.0.0.1:8006/api2/json/version >/dev/null 2>&1 ||
  { echo "fakepve did not become ready after 10s" >&2; exit 1; }

# name:cols:rows:width:height. Width/height are pre-calibrated on this
# machine (vhs 0.11.0, FontSize 14) starting from W=cols*8.4, H=rows*17 and
# nudged until the calibration tape prints exactly "cols rows"; the runtime
# check below re-verifies this on every run and aborts with the measured
# numbers if font rendering drifts on another machine.
PRESETS=(
  "80x24:80:24:750:408"
  "100x30:100:30:930:492"
  "120x40:120:40:1108:652"
)

# Steps in wizard order (wizard.DefaultConfig); must match the Screenshot
# filenames baked into wizard.tape.in.
STEP_NAMES=(welcome distribution proxmox basics node-placement networking resources addons files advanced review)

render_tape() {
  local template="$1" name="$2" w="$3" h="$4" dest="$5"
  # @HOME@ uses a `|` delimiter since $OKDCTL_DEMO_HOME itself contains `/`.
  sed -e "s/@W@/$w/g; s/@H@/$h/g; s/@NAME@/$name/g" -e "s|@HOME@|$OKDCTL_DEMO_HOME|g" "$template" > "$dest"
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
  for attempt in 1 2 3; do
    local cwd="$WORK/cwd-$name-$attempt"
    mkdir -p "$cwd"
    local tape="$WORK/wizard-$name.tape"
    render_tape "$SCREENSHOT_DIR/wizard.tape.in" "$name" "$w" "$h" "$tape"

    echo "rendering $name (attempt $attempt)..."
    (cd "$SCREENSHOT_DIR" && OKDCTL_DEMO_CWD="$cwd" vhs "$tape" >/dev/null 2>&1) || true

    local missing=()
    local step
    for step in "${STEP_NAMES[@]}"; do
      local png="$OUT_DIR/$name-$step.png"
      [ -s "$png" ] || missing+=("$step")
    done

    if [ "${#missing[@]}" -eq 0 ]; then
      echo "rendered $name: 11/11 screenshots"
      return 0
    fi
    echo "  missing after attempt $attempt: ${missing[*]}"
  done

  echo "failed to render all screenshots for $name after 3 attempts; missing: ${missing[*]}" >&2
  return 1
}

for preset in "${PRESETS[@]}"; do
  IFS=':' read -r name cols rows w h <<< "$preset"
  calibrate "$name" "$cols" "$rows" "$w" "$h"
  render_wizard "$name" "$w" "$h"
done

echo "done: $OUT_DIR ($(find "$OUT_DIR" -name '*.png' | wc -l | tr -d ' ') PNGs)"
