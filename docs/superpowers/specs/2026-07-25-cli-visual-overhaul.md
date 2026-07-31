# okdctl CLI Visual & Interaction Overhaul

**Date:** 2026-07-25
**Status:** Partially implemented — the quick wins and medium increments of §4
(color-profile gating, brand box chrome, unified glyphs, error summary box,
empty states, the shared table primitive) have landed on `develop`; the deeper
refactors (branded help template, atomic per-run log format, full theme
wiring) remain open. See `git log` for the implementing commits.
**Method:** Screenshot-driven. Every surface below was run live against the
`grappleberry` cluster from the bastion (`okdadmin@192.168.227.20`,
`~/okd-proxmox-cli`, proxmox env sourced), captured verbatim with ANSI
preserved to judge color, then stripped for this doc. Cluster access was
read-only.

---

## 0. TL;DR — the five things that matter

1. **Errors have no design.** A deploy *failure* gets a gorgeous boxed
   `FailureSummary`; an ordinary command error (`node add`, `node remove`)
   gets a raw structured log line — `[ERROR] command failed err="config
   error: …"` — with the one actionable sentence buried inside an `err=`
   attribute. This is the single biggest gap: the most stressful moment in the
   CLI is the least designed one.
2. **Two color renderers with two different rulebooks.** `charm/log` (the log
   level badges) correctly strips color when stderr is not a TTY. The lipgloss
   box helpers **always emit 24-bit truecolor** — `NO_COLOR=1` is ignored and
   piping to a file keeps every escape sequence. That's a real accessibility /
   pipe-hygiene bug, and it means "does okdctl respect the terminal" has two
   contradictory answers depending on which subsystem you hit.
3. **Boxes blow their own width budget.** `DefaultBoxWidth = 90`, but a single
   long KV *value* expands the whole box: the resize dry-run renders **131
   columns wide** because one value is a full sentence. Status is 92 wide.
   Nothing wraps; the box just grows. Widths are therefore inconsistent
   surface-to-surface and overflow narrow terminals.
4. **The brand is absent from the chrome.** The palette leads with
   Purple `#9333EA`, and the wizard logo uses it — but every box draws its
   border in slate grey and its title in near-white. The brand color only
   appears on interior section labels. Boxes read as generic, not okdctl.
5. **Two success checkmarks, two "info" colors.** CLI success is `✔` (U+2714,
   heavy); the wizard uses `✓` (U+2713, light) for the same semantic. INFO log
   badges are cyan (`#06B6D4`) while the palette's semantic `ColorInfo` is blue
   (`#3B82F6`) and goes unused. Small drifts, but they're everywhere and they
   add up to "assembled" rather than "designed."

The good news: the foundation is strong. The `render.Builder` +
`BoxedSectionCompact` + dotted-leader system is genuinely nice, the deploy
`FailureSummary`/`InterruptSummary` boxes are well-considered, and the wizard
header/scroll-indicator work is polished. The problem is **coverage and
consistency**, not taste. Most of the fixes are centralizing decisions that
are currently made per-call-site.

---

## 1. Inventory of visual surfaces

Each surface below is a captured "screenshot" (ANSI stripped). Color notes are
in brackets.

### 1.1 `okdctl status` — cluster status box

```
╭─────────────────────────────────────  CLUSTER STATUS  ─────────────────────────────────────╮
│                                                                                          │
│  cluster                                                                                 │
│    phase .................................... Running                                    │
│                                                                                          │
│  api                                                                                     │
│    reachable ................................ yes                                        │
│                                                                                          │
│  nodes                                                                                   │
│    NAME                                         ROLE    READY                            │
│    grappleberry-master0.grappleberry.k8s.local  master  yes                              │
│    grappleberry-master1.grappleberry.k8s.local  master  yes                              │
│    grappleberry-master2.grappleberry.k8s.local  master  yes                              │
│                                                                                          │
│    masters .................................. 3                                          │
│    workers .................................. 0                                          │
│    total .................................... 3                                          │
│                                                                                          │
│  cluster operators                                                                       │
│    degraded ................................. 0 (all healthy)                            │
│                                                                                          │
│                                                                                          │
╰──────────────────────────────────────────────────────────────────────────────────────────╯
```

[border slate `#334155`; title bold slate `#CBD5E1`; section labels bold
purple `#9333EA`; leader keys slate `#94A3B8`; dots slate `#334155`; values
slate `#F1F5F9`. The `NAME/ROLE/READY` table is **plain uncolored text** — no
leader, no style.]

Notes:
- Two blank rows at the bottom, one at the top (asymmetric padding).
- The `nodes` section mixes **two layouts under one header**: a raw
  column table, then three dotted-leader count rows (`masters/workers/total`).
- FQDN node names are 43 chars and dominate the box; the box widens to fit them.

### 1.2 `okdctl node resize masters --memory-mb 28672 --cpu 8 --dry-run`

```
╭───────────────────────────────────────────────────────────  NODE RESIZE  ───────────────────────────────────────────────────────────╮
│                                                                                                                                   │
│  dry-run — no changes made                                                                                                        │
│                                                                                                                                   │
│    cluster .................................. grappleberry                                                                        │
│    operation ................................ resize                                                                              │
│    target memory ............................ 28672 MiB                                                                           │
│    target cpu ............................... 8 vCPU                                                                              │
│    disruption ............................... each node is drained, then hard power-cycled (stop→start) to realize the change     │
│                                                                                                                                   │
│  nodes                                                                                                                            │
│    grappleberry-master0.grappleberry.k8s.local ... master  module.okd_cluster.proxmox_virtual_environment_vm.master[0]  [update]  │
│    grappleberry-master1.grappleberry.k8s.local ... master  module.okd_cluster.proxmox_virtual_environment_vm.master[1]  [update]  │
│    grappleberry-master2.grappleberry.k8s.local ... master  module.okd_cluster.proxmox_virtual_environment_vm.master[2]  [update]  │
│                                                                                                                                   │
│  next steps                                                                                                                       │
│    re-run without --dry-run to execute                                                                                            │
│                                                                                                                                   │
│                                                                                                                                   │
╰───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
```

[warning line `dry-run — no changes made` bold amber `#F59E0B`; `--dry-run`
inline cyan `#22D3EE`; same leader/section palette as status.]

Notes:
- **131 columns wide** — the `disruption` sentence and the full terraform
  addresses force it. This is much wider than the 92-col status box.
- The `disruption` value is a full prose sentence living in a KV *value* slot.
- Per-node lines pack three facts (`role  tf-address  [action]`) space-separated
  with no column alignment.

### 1.3 `okdctl node add --role worker --count 1 --dry-run` — ERROR state

stdout: *(empty)*, exit code `2`.
stderr (piped / JSON auto-selected):
```
[INFO] okdctl: started run_id=ZAYO6L4AZ67FRRZMQAX37WJE2Q argv="node add --role worker --count 1 --dry-run"
{"level":"error","msg":"command failed","run_id":"ZAYO6L4AZ67FRRZMQAX37WJE2Q","err":"config error: ignition tls cert not found at /home/okdadmin/okd-proxmox-cli/certs/ignition/server.crt; re-run setup (e.g. 'okdctl deploy') to regenerate it before adding a node"}
```

stderr (TTY / text format):
```
[INFO]  okdctl: started run_id=… argv="node add --role worker --count 1 --dry-run"
[INFO]  using credentials source="environment variables"
[WARN]  PROXMOX_VE_ENDPOINT not set; endpoint falling back to config file (mixed source)
[WARN]  proxmox: TLS verification disabled (insecure=true in config)
[INFO]  host memory budget node=pve total_mib=94217 allocated_mib=96256
[INFO]  datastore headroom name=local-lvm free_gib=987 total_gib=1709
[ERROR] command failed err="config error: ignition tls cert not found at /home/okdadmin/okd-proxmox-cli/certs/ignition/server.crt; re-run setup (e.g. 'okdctl deploy') to regenerate it before adding a node"
[INFO]  okdctl: finished run_id=… duration=3.182s exit_code=2
```

[INFO badge cyan `#06B6D4` bold; WARN amber; ERROR red — all via charm/log's
256-color rendering, *not* the lipgloss truecolor palette.]

Notes:
- **No box, no design.** The single actionable sentence is trapped inside
  `err="…"`.
- **Mixed log formats in one piped run:** `started`/`finished` render as
  bracket text; the error renders as JSON. (See §2.6.)
- The two `[WARN]` lines (endpoint fallback, TLS insecure) fire on *every*
  invocation and are noise for a routine op.

### 1.4 `okdctl node remove grappleberry-worker0 --dry-run` — ERROR state

```
[ERROR] command failed err="config error: node \"grappleberry-worker0\" not found in cluster; run 'okdctl node list' to list nodes"
```
Exit code `2`. Same shape as 1.3 — good actionable message, zero presentation.

### 1.5 `okdctl node list`

```
NAME                                         ROLE    READY  TF-INDEX  DRIFT  OP
grappleberry-master0.grappleberry.k8s.local  master  yes    0         none   -
grappleberry-master1.grappleberry.k8s.local  master  yes    1         none   -
grappleberry-master2.grappleberry.k8s.local  master  yes    2         none   -

DRIFT compares config sizing to terraform.tfvars on disk, not live VM state.
```

[Entirely uncolored. No box, no header styling, no zebra. A footnote line
below.] This is a *third* table style (status has one, resize has another,
this is plain columns) — none share a renderer.

### 1.6 `okdctl --help` / `okdctl node --help` / `okdctl node resize --help`

Stock cobra. Uncolored, unstyled, flat. `Available Commands:` / `Flags:` /
`Global Flags:` headers, two-space indent. No brand, no color, no visual
relationship to the boxed surfaces. `node resize --help` has excellent prose
but some flag descriptions run 140+ chars on one line.

Also: `okdctl version` and `--help` both emit a leading
`[INFO] okdctl: started run_id=…` line to stderr before the payload — log
chatter bracketing a trivial informational command.

### 1.7 `okdctl version`

```
[INFO] okdctl: started run_id=3GLYDW2HXUYQAANHNKPQUUCZWE argv=version
okdctl 0.1.0
Git Commit: unknown
Build Date: unknown
Go Version: go1.26.5
Platform:   linux/amd64
```

[Payload is plain `text/tabwriter`-style. `Key: value` with a colon, not the
dotted-leader convention used everywhere else. No brand.]

### 1.8 Spinner / progress (`internal/tui/spinner.go`, `stepprogress.go`)

Spinner line (rewriting, stderr, 120 ms tick):
```
⣾ waiting for cluster operators to stabilize (1m12s)
```
[frame cyan `#06B6D4` bold; braille frames `⣾⣽⣻⢿⡿⣟⣯⣷`.]

Deploy step checklist (`StepProgress`):
```
[1/9] render ignition · setup            ✔ (2.104s)
[2/9] upload ISO · setup                 ✔ (8.921s)
[3/9] apply terraform · install          ✔ (1m3s)
[4/9] wait for bootstrap · install       ↷ skipped
[5/9] monitor install · install          ✖ (4m2s)
```
[in-progress line dim slate `#64748B`; done rows white; `✔` green `#22C55E`,
`✖` red `#EF4444`, `↷ skipped` dim. Counter `[N/total]`.]

### 1.9 Error taxonomy (`internal/errtypes`)

Five typed errors — `ConfigError` (exit 2), `NetworkError` (3),
`ClusterError` (4), `AuthError` (5), `UsageError` (64). `Error()` renders
`"<kind> error: <msg>"`. Solid taxonomy; **never surfaced visually** — it's
only ever stringified into an `err=` log attr (§1.3). The typed distinction
that the code carefully preserves is invisible to the user.

### 1.10 Deploy wizard (`internal/tui/wizard/`) — reconstructed from source

Full-screen bubbletea alt-screen app. Reconstructed frame:

```
┌──────────────────────────────────────────────────────────────────────┐
│ O K D C T L                                                            │
│ okd over proxmox, the easy way              ●─●─○─○─○  step 2 of 5     │
│────────────────────────────────────────────────────────────────────── │
│                                                                        │
│   Cluster basics                                                       │
│                                                                        │
│   Cluster name  [ grappleberry                    ]                    │
│   Base domain   [ k8s.local                        ]                   │
│                                                                        │
│                                                                        │
│──────────────────────────  50% • scroll for more  ───────  ▸ okd 4.15  │
│  ↑↓ navigate    enter confirm    esc back    ctrl+c quit               │
└──────────────────────────────────────────────────────────────────────┘
```

[outer border slate `#475569`; logo bold purple; tagline italic slate `#94A3B8`;
progress dots — completed `●` green, current `●` purple, pending `○` slate;
help-bar keys are dark-on-slate-300 chips; context badge `▸ okd 4.15` bold
green.] Section completion inside the data-driven flow uses `✓` (light check) —
**different glyph from the CLI's `✔`.** This is the most polished surface in the
product and, notably, the only place the brand purple touches structural chrome.

---

## 2. Critique — where it diverges

### 2.1 Color gating is inconsistent and non-compliant
`charm/log` respects TTY detection (level badges go plain when piped);
lipgloss box helpers do not. Verified:

```
$ NO_COLOR=1 okdctl status | cat -v
… ^[[38;2;71;85;105m … ^[[1;38;2;203;213;225m CLUSTER STATUS ^[[m …
```

Truecolor escapes survive `NO_COLOR=1` **and** a non-TTY pipe. lipgloss v2's
renderer is never told the output profile, so it defaults to full truecolor
unconditionally. This breaks `NO_COLOR`, breaks logs-to-file, and breaks dumb
terminals. It's the highest-severity finding because it's a correctness/
accessibility issue, not just aesthetics.

### 2.2 Errors are undesigned — the biggest UX gap
The whole `errtypes` taxonomy and the carefully-written actionable hints
(`re-run setup …`, `run 'okdctl node list' …`) are delivered as a log line
with the payload inside `err=`. There is a beautiful box renderer three
imports away (`render.FailureSummary`) that never gets called for
non-deploy command errors. The most stressful surface is the least designed.

### 2.3 Box width is content-driven, not budgeted
`boxedSectionCore` computes `innerWidth = max(width-2, maxContentWidth+2, …)`.
Any single long value wins, so a prose `disruption` value or a full terraform
address balloons the box to 131 cols. Consequences: inconsistent widths
(status 92, resize 131), horizontal overflow in an 80- or 100-col terminal,
and no relationship to the declared `DefaultBoxWidth = 90`. Nothing wraps.

### 2.4 Three table styles, no shared renderer
`status` node table, `resize` per-node lines, and `node list` are three
different hand-rolled column layouts. Only one (`node list`) has extra columns;
none is colored consistently; the status table header is unstyled while its
surroundings are colored. There is no `tui.Table` primitive.

### 2.5 Iconography and semantic-color drift
- Success: `✔` (CLI) vs `✓` (wizard) for the same meaning.
- INFO: badge cyan `#06B6D4`, but palette `ColorInfo` = blue `#3B82F6`
  (defined, unused for the badge). Spinner also cyan. So "cyan" doubles as
  both "info" and "in-progress," and the declared info-blue never appears.
- Brand purple is a section-label color only; box borders/titles are slate.
  The product's signature color is nearly invisible outside the wizard.

### 2.6 Log format is not atomic per run
When stderr is piped, `started`/`finished` render as bracket text but the
error record renders as JSON — two formats in one run's output. A machine
consumer doing `2>&1 | jq` chokes on the non-JSON lines; a human reading a
redirected log sees a format switch mid-stream. The `started`/`finished`
bookends should honor the same formatter as everything else (or move to Debug).

### 2.7 Vertical rhythm and micro-spacing
- Boxes: 1 blank row top, 2 blank rows bottom (asymmetric).
- `version` uses `Key: value` colons; everything else uses dotted leaders.
- Routine node ops print two `[WARN]` lines (endpoint fallback, TLS insecure)
  every single time — warning fatigue that trains operators to ignore WARN.
- `run_id` ULIDs are shown in interactive INFO lines; useful for bug reports,
  noise for a human at a prompt.

### 2.8 Empty & degraded states are thin
`status` with zero nodes prints `no nodes reported` as a bare indented string
(no icon, no framing). `node list` empty state is unexamined. `plan` no-drift
is a one-liner in a box. Empty states are where a premium CLI reassures;
here they're afterthoughts.

### 2.9 Help is off-brand
Stock cobra help is the most-hit surface for new users and shares *nothing*
with the designed boxes — no color, no brand, no leader typography. The jump
from `--help` to `status` feels like two different tools.

---

## 3. Proposed cohesive visual system

### 3.1 Palette — keep the hexes, fix the *roles*
The literal hexes in `colors.go` are good (Tailwind-derived, coherent). The
problem is role assignment, not the swatches. Lock these semantic roles and
use them *everywhere*, including box chrome and log badges:

| Role | Hex | Used for |
|------|-----|----------|
| `Primary` (purple) | `#9333EA` | brand: box titles, section labels, wizard logo, primary emphasis |
| `PrimaryDim` | `#6B21A8`* | box borders (brand-tinted, not grey) |
| `Success` (green) | `#22C55E` | ✔, ready, completed, no-drift |
| `Warning` (amber) | `#F59E0B` | ⚠, dry-run, drift, interrupted |
| `Error` (red) | `#EF4444` | ✖, failures, degraded, not-ready |
| `Info` (blue) | `#3B82F6` | INFO badge (retire cyan-for-info) |
| `Accent` (cyan) | `#06B6D4` | spinner + inline code *only* (in-progress motion) |
| `TextDim` (slate-400) | `#94A3B8` | leader keys, taglines |
| `Leader` (slate-700) | `#334155` | leader dots, rules |

\* add one tint; or reuse `#7E22CE`. The single most impactful visual change
is **tinting box borders + titles with the brand**, so every box reads as
okdctl at a glance. Decision: title in `Primary`, border in `PrimaryDim`.

**All color must flow through a renderer that is told the output profile.**
Detect once at startup (TTY? `NO_COLOR`? `CLICOLOR_FORCE`? `--no-color`?),
set the lipgloss default renderer's color profile accordingly, and let every
`style.Render` downgrade automatically. This fixes §2.1 globally.

### 3.2 Box convention — one primitive, budgeted width, wrapping values
- **Title:** bold `Primary`, single-line compact header (keep the current
  compact style — it's nice).
- **Border:** `PrimaryDim` rounded.
- **Width:** honor `DefaultBoxWidth` as a *ceiling clamped to terminal width*.
  Long values **wrap to the value column**, they do not expand the box.
- **Padding:** symmetric — 1 blank row top and bottom.
- **Prose out of KV slots:** sentences like `disruption: each node is drained…`
  become a labeled note line under the section, wrapped, not a leader value.

Before (resize, 131 cols) → After (90 cols, brand border, wrapped prose):

```
╭──────────────────────────────  NODE RESIZE  ──────────────────────────────╮   ← purple title + border
│                                                                            │
│  ⚠ dry-run — no changes made                                               │
│                                                                            │
│  plan                                                                      │
│    cluster ....................... grappleberry                            │
│    target memory ................. 28672 MiB                               │
│    target cpu .................... 8 vCPU                                   │
│    disruption .................... each node is drained, then hard         │
│                                    power-cycled (stop→start) to realize    │
│                                    the change                              │
│                                                                            │
│  nodes                                                                     │
│    NODE                    ROLE    ADDRESS               ACTION            │
│    grappleberry-master0    master  …vm.master[0]         update            │
│    grappleberry-master1    master  …vm.master[1]         update            │
│    grappleberry-master2    master  …vm.master[2]         update            │
│                                                                            │
│  next steps                                                                │
│    → re-run without --dry-run to execute                                   │
│                                                                            │
╰────────────────────────────────────────────────────────────────────────────╯
```

(Node names shortened to the short hostname; the FQDN suffix is redundant
inside a single-cluster box. Terraform addresses middle-truncated to the
distinguishing tail.)

### 3.3 Error treatment — the same box, an error skin
Route **every** top-level command error through a `render.ErrorSummary(err)`
that:
1. reads the `errtypes` kind → colored kind chip (`CONFIG`, `CLUSTER`, …);
2. renders the message as the headline;
3. surfaces the actionable hint (already in the message after the last `; `) as
   a `→ next step` line;
4. shows exit code and `run_id` in a dim footer for bug reports.

Before → After:

```
[ERROR] command failed err="config error: ignition tls cert not found at
/home/…/certs/ignition/server.crt; re-run setup (e.g. 'okdctl deploy') to
regenerate it before adding a node"
```

```
╭──────────────────────────────────  ERROR  ────────────────────────────────╮   ← red title + border
│                                                                            │
│  ✖  config error                                                           │
│                                                                            │
│  ignition tls cert not found at                                            │
│  /home/okdadmin/okd-proxmox-cli/certs/ignition/server.crt                  │
│                                                                            │
│  → re-run setup to regenerate it:  okdctl deploy                           │
│                                                                            │
│  exit 2 · run_id 3GLYDW2HXUYQAANHNKPQUUCZWE                                 │
│                                                                            │
╰────────────────────────────────────────────────────────────────────────────╯
```

The `[ERROR] command failed …` log line stays for the file sink / JSON
consumers (machine surface), but the **human TTY sees the box**. Gate on
`ProgressBarsEnabled()` exactly like the deploy summaries already do.

### 3.4 Empty-state treatment
Give empty states a consistent glyph + reassurance, not a bare string:

```
│  nodes                                                                     │
│    ○ no worker nodes yet — add one with  okdctl node add                   │
```

### 3.5 Spinner / progress — keep, unify color
The braille spinner and the step checklist are good. Two tweaks: (a) spinner
stays `Accent` cyan (motion), (b) unify the completion glyph to a single
success mark across CLI and wizard (pick `✔`; §3.6). Consider a subtle
`Success`-green fade on the final frame before the checklist commit.

### 3.6 Iconography — one set, documented
Freeze one glyph table in `icons.go` and delete the wizard's local `✓`/`●`
literals in favor of it:

| Meaning | Glyph | Color |
|---------|-------|-------|
| success / ready / complete | `✔` | Success |
| error / not-ready / failed | `✖` | Error |
| warning / dry-run / drift | `⚠` | Warning |
| skipped | `↷` | Dim |
| pending / empty | `○` | Slate-600 |
| current / active | `●` | Primary |
| next-step / pointer | `→` | Primary |
| in-progress | braille frames | Accent |

### 3.7 Table primitive
Add `tui.Table(headers, rows, opts)` that: styles the header in `TextDim`,
right-pads columns from widest plain cell (the existing status logic, extracted),
optionally colors a whole row by state (red for not-ready), and middle-truncates
over-long cells to a max column width. `status`, `node list`, and the resize
node section all consume it — one table look everywhere.

### 3.8 Help styling
Apply a cobra help template that: colors the `Usage:`/`Available Commands:`/
`Flags:` headers in `Primary`, dims flag defaults, and prints a one-line
branded header (`okdctl — okd over proxmox`). Keep it terse; this is a lipgloss
template pass, not a re-architecture. Even minimal color pulls help into the
system.

---

## 4. Concrete changes mapped to code

### Quick wins (hours, low risk)

| # | Change | Files |
|---|--------|-------|
| Q1 | **Set the lipgloss color profile from output detection** (TTY / `NO_COLOR` / `CLICOLOR_FORCE`); one call in startup. Fixes §2.1 everywhere. | `internal/tui/logger.go` (or new `colorprofile.go`), wired in `internal/cli/root.go::PersistentPreRunE` |
| Q2 | **Tint box border + title with the brand** (title `Primary`, border `PrimaryDim`). | `internal/tui/layouts.go` (`BoxedSectionCompact` cfg), `colors.go` (add `PrimaryDim`) |
| Q3 | **Unify success glyph** — wizard uses `tui.IconSuccess`, delete local `✓`. | `internal/tui/wizard/datadriven.go`, `steps/review.go` |
| Q4 | **INFO badge → `ColorInfo` blue**, free cyan for motion/code only. | `internal/tui/logger.go::buildLogger` |
| Q5 | **Symmetric box padding** (1 top / 1 bottom); audit trailing `Newline()`s. | `internal/render/{summary,nodeop,plan}.go` |
| Q6 | **Demote the two per-run WARNs** (endpoint fallback, TLS insecure) to once-per-run or Debug on routine ops; demote `started`/`finished` to Debug for non-deploy verbs. | credential/config load sites; `internal/cli/root.go::execute` |
| Q7 | **`version` uses dotted leaders**, not `Key:` colons. | `internal/cli/version.go` |

### Medium (a day each)

| # | Change | Files |
|---|--------|-------|
| M1 | **`render.ErrorSummary(err)` box** + route it from `execute()` for TTY. Reads `errtypes` kind → chip, splits hint on last `; `, shows exit/run_id. §3.3. | new `internal/render/errorbox.go`; `internal/cli/root.go::execute` (§1.3 path); consume `errtypes` |
| M2 | **Width budget + value wrapping** in the box core: clamp `innerWidth` to `min(width, termWidth)`, wrap long values into the value column. §2.3/§3.2. | `internal/tui/layouts.go::boxedSectionCore`, `internal/tui/helpers.go::dottedKV` |
| M3 | **`tui.Table` primitive**, extract from `nodeStatusTableLines`; adopt in status/list/resize. §3.7. | new `internal/tui/table.go`; `internal/cli/status.go`, `node list` cmd, `internal/render/nodeop.go` |
| M4 | **Prose-out-of-KV** — a `sb.Note(label, text)` Builder method that wraps; move `disruption` and long next-steps onto it. | `internal/render/summary.go` (Builder), `nodeop.go` |
| M5 | **Empty-state helper** `tui.EmptyState(glyph, msg, hint)`; adopt in status/list. §3.4. | `internal/tui/rendering.go`; call sites |

### Deeper refactors (multi-day, sequence last)

| # | Change | Files |
|---|--------|-------|
| D1 | **Branded cobra help template** across all commands. §3.8. | `internal/cli/root.go` (SetHelpTemplate/SetUsageTemplate), shared template pkg |
| D2 | **Atomic per-run log format** — one formatter for the whole run incl. bookends; make `2>&1 \| jq` clean under `--output json`. §2.6. | `internal/tui/logger.go`, `internal/cli/root.go` |
| D3 | **Theme wiring** — the `ThemeHighContrast` scaffold exists; finish it and add `--no-color`/profile as first-class, plus the `okdctl theme` verb the code already anticipates. | `internal/tui/colors.go`, root flags |

---

## 5. Incremental implementation plan

**Phase 0 — foundation (unblocks correctness).** Q1 (color profile) first;
it's the one change that touches every surface and fixes the `NO_COLOR`/pipe
bug. Land Q4 alongside (badge color) since both live in the logger.

**Phase 1 — cheap coherence.** Q2, Q3, Q5, Q7 — brand-tinted boxes, one
checkmark, symmetric padding, leader-style `version`. Purely visual, each
independently shippable, together they make the product read as designed. Q6
(log-noise) rides along.

**Phase 2 — the error box.** M1. Highest user-visible payoff after Phase 0.
Reuses the deploy-summary pattern, so it's a well-trodden path. Ship with
golden tests over the `errtypes` kinds.

**Phase 3 — layout discipline.** M2 (width/wrap) then M3 (table) then M4
(prose) then M5 (empty states). M2 before M3 because the table primitive should
inherit the wrapping/truncation rules. After this, every box obeys one width
and every table one look.

**Phase 4 — polish & theming.** D1 (help), D2 (log atomicity), D3 (finish the
theme scaffold + `--no-color`). Lower urgency, higher blast radius — sequence
last.

**Testing throughout.** The codebase already golden-tests renders
(`nodeop_test.go`, `summary_test.go`, `plan_test.go`). Every change here should
add/adjust a golden. For the color-profile work, add a test asserting
`NO_COLOR=1` yields zero escape sequences from `BoxedSectionCompact` — that's
the regression that must never come back.

---

## Appendix — current visual system catalog

**Palette** (`internal/tui/colors.go`): purple `#9333EA` (Primary), green
`#22C55E` (Success), amber `#F59E0B` (Warning), red `#EF4444` (Error), blue
`#3B82F6` (Info, mostly unused), cyan `#22D3EE`/`#06B6D4` (inline code /
spinner), slate ramp `#F1F5F9 → #0F172A`. High-contrast variant exists
(`ThemeHighContrast`, env-gated) but the `okdctl theme` verb is unbuilt
scaffolding.

**Box** (`internal/tui/layouts.go`): rounded `╭─╮ │ ╰─╯`; compact = single-line
centered title in the top border; border `ColorSlate600`, title `ColorSlate300`;
`DefaultBoxWidth = 90` but width grows to fit content.

**Leaders** (`internal/tui/helpers.go`): `key ....... value`; key `Slate400`,
dots `Slate700`, value `Slate100` (or amber-bold when highlighted); floor of 3
dots.

**Icons** (`internal/tui/icons.go`): `✔ ✖ ⚠ ↷`. Wizard adds `✓ ● ○ ▸ ↑ ↓ ◂ ▸`
locally (note the `✓`/`✔` split).

**Spinner** (`internal/tui/spinner.go`): braille `⣾⣽⣻⢿⡿⣟⣯⣷`, 120 ms, cyan-bold,
single owned line via `lineowner`.

**Logger** (`internal/tui/logger.go`): charm/log behind slog + `RedactHandler`;
level badges `[DEBUG]`(slate) `[INFO]`(cyan) `[WARN]`(amber) `[ERROR]`(red);
respects TTY for its own color (unlike the boxes).

**Errors** (`internal/errtypes`): 5 typed errors → structured exit codes;
`"<kind> error: <msg>"`; surfaced only as an `err=` log attr today.

**Wizard** (`internal/tui/wizard/`): bubbletea alt-screen; slate-bordered frame,
purple `O K D C T L` logo, italic tagline, `●/○` progress dots, chip-style
help bar, dashed scroll indicator with centered `%` and a green context badge.
The most polished surface — and the only one that puts the brand on the chrome.
