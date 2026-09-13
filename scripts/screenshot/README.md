# Wizard screenshots

`make screenshots` (or `scripts/screenshot/run.sh` directly) renders the
11-step `okdctl deploy` wizard against the fakepve fixture and screenshots
the header of every step, at three terminal sizes: 80x24, 100x30, 120x40.
Requires `vhs` (`brew install vhs`) and `go`; nothing touches a real
hypervisor or deploys.

Output lands in `scripts/screenshot/out/` (gitignored): 33 PNGs
(`<size>-<step>.png`, e.g. `80x24-basics.png`) plus a `<size>.txt` raw
terminal capture and `calib-<size>.txt` calibration readout per size.

## How it works

- `calibrate.tape.in` is a template that prints `tput cols`/`tput lines` at
  a given pixel size, so `run.sh` can confirm a preset's `Set Width`/`Set
  Height` actually maps to its named column/row count before rendering.
- `wizard.tape.in` is a template that drives the full wizard walkthrough —
  `okdctl deploy` against the fake Proxmox API from `scripts/demo/fakepve.go`
  — with `Wait+Screen@10s /step N of 11/` gating each step transition and a
  `Screenshot` right after. Each `STEP N` block is self-contained: the
  wait/screenshot pair that opens it, then the keystrokes that complete that
  step and advance to the next. Keystrokes mirror `docs/assets/demo.tape`
  (the human-paced README recording) with sleeps compressed for unattended
  batch rendering.
- `run.sh` substitutes `@W@`/`@H@`/`@NAME@` into both templates per preset,
  builds the demo binary, starts fakepve, and renders each preset in turn.

## Width/height presets

vhs 0.11.0 has no columns/rows setting — only pixel `Set Width`/`Set
Height` — so the presets in `run.sh` are calibrated pixel values, not the
raw `cols * charwidth` arithmetic. `run.sh` re-verifies each preset's
calibration on every run (via `calibrate.tape.in`) and aborts with the
measured numbers if it drifts, e.g. on a machine with different font
rendering.

vhs's `Screenshot` command's internal frame composite has also been
observed to fail sporadically at certain pixel sizes even when the
terminal itself renders correctly (no upstream issue filed yet); `run.sh`
retries a preset's render (fresh state each time) up to 3 times before
giving up and reporting exactly which screenshots are missing.

## Regenerating after a UI change

The wizard UI is being refit incrementally; each `STEP N` block in
`wizard.tape.in` is delimited so its keystroke sequence can be updated on
its own without touching the surrounding steps.
