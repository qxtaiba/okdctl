# Wizard screenshots

`make screenshots` (or `scripts/screenshot/run.sh` directly) renders the
11-step `okdctl deploy` wizard, the hero-hub over a deployed cluster, a
distribution-step failure, and the Cluster Lifecycle wizard against the
fakepve fixture, and screenshots every step, at four terminal sizes:
80x24, 100x30, 120x40, 180x48. Requires `vhs` (`brew install vhs`) and
`go`; nothing touches a real hypervisor or deploys.

Output lands in `scripts/screenshot/out/` (gitignored): 24 PNGs per size
(`<size>-<step>.png`, e.g. `80x24-basics.png`), a `<size>.txt`,
`<size>-hub.txt`, `<size>-distribution-fail.txt`, and
`<size>-lifecycle.txt` raw terminal capture, and a `calib-<size>.txt`
calibration readout per size.

## How it works

- `calibrate.tape.in` is a template that prints `tput cols`/`tput lines` at
  a given pixel size, so `run.sh` can confirm a preset's `Set Width`/`Set
  Height` actually maps to its named column/row count before rendering.
- `wizard.tape.in` is a template that drives the full wizard walkthrough —
  `okdctl deploy` against the fake Proxmox API from `scripts/demo/fakepve.go`.
  The wait gates match the header's phase trail (`connect · cluster ·
  extras · review`), not a step count: each step renders a distinct trail
  string — its phase bolded, plus an inline dot ribbon once that phase has
  2 or more visible steps — and that string, unlike the step title, is
  never clipped by the header's width budget, so it gates reliably at
  every preset size. Each `STEP N` block is self-contained: the
  wait/screenshot pair that opens it, then the keystrokes that complete that
  step and advance to the next. Keystrokes mirror `docs/assets/demo.tape`
  (the human-paced README recording) with sleeps compressed for unattended
  batch rendering.
- `hub.tape.in` walks the hero-hub's own deployed-state screens: the
  launcher, `cluster status` and its node-detail drill-in, and the
  `manage nodes` operation picker, seeded with a fake Terraform state so
  the hub's save-slot line reads `deployed`.
- `distribution-fail.tape.in` captures the distribution step's error state
  under `OKDCTL_DEMO_RELEASES=fail` — the empty state and retry ribbon,
  with no interaction beyond opening the step.
- `lifecycle.tape.in` walks `okdctl node manage`'s remove-worker flow
  against `lifecycle.DemoHooks`' static six-node fixture, through all
  seven of its screens.
- `run.sh` substitutes `@W@`/`@H@`/`@NAME@` into every template per preset,
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
