# holistic-review and roadmap-ideate — skill design

**Date:** 2026-04-26
**Status:** approved (brainstorming)
**Skills:** `holistic-review`, `roadmap-ideate`
**Install path:** `~/.claude/skills/<name>/SKILL.md`

## Why

`/audit-all` is finding-driven: 14 specialist audits produce schema-validated
JSONL atoms with `file:line` evidence. It cannot ask "does the *shape* of
this codebase make sense?" because the answer isn't an atom — it's a
judgment.

These skills fill that gap.

- **holistic-review** — principal-engineer critique of what's *here*: where
  the architecture is rough, where AI slop accumulated, where docs claim
  one thing and the code does another. Produces roadmap items in a new
  tier.
- **roadmap-ideate** — staff-engineer brainstorm of what should come
  *next*: feature gaps, big-arch refactors, UX investments, ecosystem
  positioning. Produces roadmap items in a new tier.

Both feed `/roadmap-pickup`, which executes claimed items.

## Boundary between the two

| | holistic-review | roadmap-ideate |
|---|---|---|
| Question | "what's wrong with what's here?" | "what's missing for the future?" |
| Item flavor | fix / refactor / cleanup | feature / capability / forward investment |
| API stability lives in | — | yes (1.0-readiness is forward-looking) |
| Architectural debt | yes (current shape problems) | yes (refactors that unlock new capability) |

When in doubt, the difference is direction: holistic looks backward (what
got built), ideate looks forward (what should come next).

## Design constraints (durable, bind both skills)

1. **No `/audit-all` overlap.** No file:line atoms; report patterns,
   shapes, and judgments. Findings that fit the audit schema belong in
   `/audit-all`.
2. **Don't violate Product philosophy.** The Product philosophy section
   of `roadmap.md` (single provider, Linux-only, etc.) is hard
   constraint. Items reintroducing skipped scope (multi-provider, RKE2,
   Windows) must not be proposed.
3. **Respect `MEMORY.md` carve-outs.** Especially scaffolding: code
   shaped like future CLI commands stays even if `deadcode` flags it.
4. **Two-phase: autonomous draft, then interactive jam.** Phase 1
   parallel agents produce a candidates JSON. Phase 2 is user-driven
   selection over `AskUserQuestion`.
5. **Fresh run every time.** Files are timestamped; no resumability.
6. **No push.** Skills commit; user pushes.
7. **Tier-letter assignment.** Skills auto-detect the next free tier
   letter by greppping `### Tier X —` headers in `roadmap.md`.

## Skill alignment with existing okdctl skill conventions

`SKILL.md` files for both skills must mirror the shape used by
`audit-all/SKILL.md` and `roadmap-pickup/SKILL.md`:

- **Frontmatter:** `name:` + rich `description:` ending in trigger
  phrases (e.g., "Use whenever the user says X, Y, or Z").
- **Top-of-file:** one-line summary; "Read first:" line listing the
  files to load before dispatch.
- **Numbered top-level sections:** `## 0)` or `## 1)` onward, never
  hierarchical bullets at the top level.
- **Embed hard constraints in agent prompts** — the
  Cloudflare "tell the LLM what NOT to do" pattern. Every dispatched
  agent gets a verbatim "Hard rules" block.
- **Closing sections:** `## What this skill does NOT do` /
  guardrails section before the final "When a fresh session invokes
  you" line.
- **Pre-flight read order:** `roadmap.md` → `CLAUDE.md` → `MEMORY.md`
  → `go.mod` / `README.md` → `docs/architecture/*.md`. Same shape as
  `roadmap-pickup`.
- **Commit format:** follow `CLAUDE.md`. `type(scope): description`,
  lowercase, imperative, ≤70 char subject. **No** `Co-Authored-By:`
  trailers. **No** AI/LLM references. **No** `--no-verify`.

These skills do **not** inherit `_conventions/AUDIT_CONVENTIONS.md` —
that contract is JSONL-finding-shaped (skill-prefix IDs, severity
rubric, schema validation). These skills produce JSON candidates, not
JSONL findings, so the audit convention is the wrong fit.

## Shared infrastructure

### Staging JSON shape

Path: `.claude/audits/<skill-name>-candidates-YYYY-MM-DD-HHMMSS.json`

```json
{
  "skill": "holistic-review",
  "run_iso": "2026-04-26T14:30:22Z",
  "tier_letter": "I",
  "preflight": {
    "head_sha": "<sha>"
  },
  "observations": [
    {
      "agent": "Authenticity",
      "text": "Pattern of nil-checks-on-never-nil scattered across internal/distribution. Not rolled up to a candidate because remediation is package-by-package judgment."
    }
  ],
  "candidates": [
    {
      "id_stub": "I1",
      "agent": "Structure",
      "title": "Tighten internal/addon import boundary",
      "category": "refactor / architecture",
      "state": "design needed",
      "effort": "days",
      "impact": "medium",
      "evidence": ["internal/addon/addon.go:13-19"],
      "acceptance": ["..."],
      "depends_on": [],
      "rationale": "1-3 sentence why this matters",
      "selection": "pending"
    }
  ]
}
```

`selection` lifecycle: `pending → selected | rejected | merged-into:<id>`.
The skill writes one mutation per Phase-2 step so the JSON is the
authoritative record of what the user chose.

`observations` is for non-actionable judgments — patterns the agent
noticed that don't roll up to a concrete candidate. Phase 2 surfaces
observations as read-only context during the executive summary; they
never become roadmap items.

Agents return candidates without `id_stub`; the orchestrator assigns
them in agent-output order before writing the JSON file. Phase 2 may
re-number selected items to keep them contiguous post-rejection.

### Tier-letter detection

```
grep '^### Tier [A-Z] —' roadmap.md | awk '{print $3}' | sort -u
```

Pick the next unused letter. If `Z` is taken, fail loudly — the skill
does not roll over.

### Phase 2 — interactive selection (shared algorithm)

```
1. Read candidates JSON.
2. Show executive summary:
   - Per-agent observations (read-only).
   - Candidate table: id_stub | agent | title | effort | impact.
3. AskUserQuestion (multi-select): "Which candidates to keep?"
   → mark unchosen as rejected, chosen as selected.
4. Per selected item, in order:
   a. Show full candidate (acceptance, evidence, rationale).
   b. AskUserQuestion (single-select):
      - include as-is
      - edit (you give a one-line note → fresh agent revises)
      - merge with <other selected id>
      - drop after all
5. If zero items survive: do NOT touch roadmap.md, then nuke the JSON
   file (`rm <path>`; gitignored, no commit needed). Exit cleanly.
6. Otherwise:
   a. Assign sequential IDs (<letter>1, <letter>2, ...) in selection order.
   b. Render the new tier markdown.
   c. Show diff preview of roadmap.md change.
   d. AskUserQuestion final gate: "commit?"
   e. On yes: append tier, commit narrowly per CLAUDE.md format.
7. After successful commit (step 6e) OR zero-survivor exit (step 5),
   nuke the candidates JSON: `rm .claude/audits/<skill>-candidates-...json`.
   Roadmap is the contract; the JSON is consumed.
```

### Commit conventions

- Format per CLAUDE.md.
- Subject: `chore(roadmap): tier-<letter> <skill-name>` (e.g.,
  `chore(roadmap): tier-I holistic review`).
- Body (optional): one line per agent contributing items, count
  appended.
- Stage narrowly: `git add roadmap.md`. Never `git add -A`.
- No push. User pushes after diff review.

## Skill 1: holistic-review

### Persona

A principal engineer reading the repo cold for the first time. Not
running checklists; exercising judgment.

### Pre-flight

The orchestrator reads, in order:

1. `roadmap.md` (Product philosophy, Status lifecycle, Completed ledger).
2. `CLAUDE.md` (commit conventions, comment density, canonical helpers).
3. `MEMORY.md` (carve-outs and feedback memories).
4. `go.mod`, `README.md`, `.golangci.yml`.
5. `docs/architecture/*.md`.

No `golangci-lint` gate — even a WIP baseline is fair game for review.

### Phase 1 — 5 synthesizer agents in parallel

Each agent dispatched as `Agent` with
`subagent_type: general-purpose` (web research enabled).

| # | Theme | Lenses owned |
|---|---|---|
| 1 | **Structure** | architecture & boundaries · cross-package duplication |
| 2 | **Authenticity** | AI-generated code smell · half-done scaffolding · per-package maturity heatmap |
| 3 | **Domain coherence** | domain-model accuracy · state/lifecycle clarity · config surface coherence |
| 4 | **Operator UX** | operability & user mental model · failure-mode legibility · docs/README reality gap |
| 5 | **Test honesty + cognitive load** | test-behavior coverage realism · cognitive-load heatmap |

Web research is permitted for any agent — to validate against latest
idiomatic patterns, official documentation, current best practice.

Each agent prompt contains:

- Theme name + lenses owned.
- Orchestrator's pre-flight summary so agents skip re-reading
  `CLAUDE.md` from scratch.
- **Hard rules:**
  - Do NOT re-derive findings already in `.claude/audits/audit-*.jsonl`
    (trust the rule; do not read those files).
  - Do NOT report file:line atoms; report patterns, shapes,
    judgments. `evidence` field on a candidate is fine to include
    file:line touchstones for `/roadmap-pickup` later.
  - Do NOT propose items violating Product philosophy.
  - Respect MEMORY.md scaffolding carve-out.
- **Output contract:** JSON-shaped candidates plus inline
  `observations`. No separate markdown report. The
  `rationale` field on each candidate carries the narrative.

### AI-smell sub-signals (Authenticity agent)

Larridin / Cloudflare AI-slop signal set; agent rolls these up to
package-level judgments:

- Semantic duplication (same logic in two places that should call
  shared code).
- Defensive code for impossible states (nil-checks on never-nil values).
- Speculative configuration (knobs nobody turns; flags with one call site).
- Generic helper packages with no real consumers.
- Comments that explain *what* in English ("set i to 0").
- Tests that mirror implementation rather than verify behavior.
- Verbose docstring fingerprint on otherwise-obvious functions.
- Premature factory/wrapper layers.

These are sub-signals the agent looks for, not a per-signal output.

### Output

- `.claude/audits/holistic-review-candidates-YYYY-MM-DD-HHMMSS.json`
  — staging file consumed by Phase 2.

### Phase 2

Shared algorithm above. Tier name on commit:
`Tier <letter> — holistic review YYYY-MM-DD`.

## Skill 2: roadmap-ideate

### Persona

A staff engineer scoping the next quarter's roadmap, working with the
maintainer to find the highest-leverage forward bets.

### Pre-flight

Same files as holistic-review. No lint gate. Skill verifies
`roadmap.md` has a Product philosophy section to bind the agents
before dispatch.

### Phase 1 — 4 axis agents in parallel

Each agent dispatched as `Agent` with
`subagent_type: general-purpose` (web research enabled).

| # | Axis |
|---|---|
| 1 | Feature gaps (capabilities Proxmox+OKD operators want that okdctl skips) |
| 2 | Architectural debt + API stability / 1.0-readiness |
| 3 | UX & operability roadmap |
| 4 | Ecosystem positioning vs analogous tools (kubespray, talos, openshift-installer, kubeadm) |

Web research is permitted for all axes — to compare against
contemporary patterns, validate ideas against industry direction, find
prior art before proposing okdctl-specific shapes.

Each agent prompt contains:

- Axis description + scope guidance.
- Orchestrator's pre-flight summary.
- **Hard rules:**
  - Items must respect Product philosophy.
  - For axis 4: web research is required, but proposed items must
    still be okdctl-shaped, not literal feature ports from other
    tools.
  - For axes 1-3: web research is permitted but the candidate's
    rationale must ground in repo state too.
- **Output contract:** 5-10 candidates each, in the staging JSON
  shape. Each candidate must include a `rationale` line that names
  the problem the item solves.

### Output

- `.claude/audits/roadmap-ideate-candidates-YYYY-MM-DD-HHMMSS.json` —
  staging file.

### Phase 2

Shared algorithm above. Tier name on commit:
`Tier <letter> — forward ideate YYYY-MM-DD`.

## Out of scope (explicit non-goals)

- **Auto-merging items into existing tiers.** Both skills always
  create a new tier.
- **JSONL audit-style output.** These skills produce JSON staging
  files, not JSONL findings. No `finding-schema.json` validation;
  that's `/audit-all` territory.
- **Push to remote.** User pushes manually.
- **PR creation.** No `gh pr create` — these aren't PR-shaped
  changes.
- **Markdown narrative report.** Dropped in favor of JSON-only output;
  the `rationale` field per candidate carries the narrative.
- **MEMORY.md mutations.** Skills do not write to MEMORY.md.
- **Modifying existing roadmap items.** Skills only append a new tier.
  Touching existing items requires user consent.
- **Tests for the skills themselves.** Skills are markdown prompts; no
  unit tests. The `/roadmap-pickup` quality gate eventually catches
  bad items via the reviewer.

## Open questions (resolved during brainstorming)

- ~~Item ID prefix scheme~~ → tier-letter-keyed (I1, J1).
- ~~Auto-append vs human gate~~ → Phase-2 multi-select selection gate.
- ~~Resume-on-same-day~~ → always fresh; timestamped files.
- ~~holistic-review vs roadmap-ideate boundary~~ → "what's wrong here"
  vs "what's missing for the future". API stability lives in ideate.
- ~~Skill names~~ → `holistic-review` and `roadmap-ideate`.
- ~~Web research scope~~ → all agents in both skills can web-research.
- ~~Output artifacts~~ → JSON-only with inline `observations` for
  non-actionable judgments. No markdown narrative.
- ~~Pre-flight lint gate~~ → neither skill gates on `golangci-lint`.
- ~~Output discipline~~ → candidates plus inline `observations`.
- ~~Phase 2 'edit' mechanism~~ → fresh agent re-dispatched on user note.
- ~~Audit-finding dedup~~ → trust the prompt rule; don't read
  `.claude/audits/audit-*.jsonl`.
- ~~Zero-survivors behavior~~ → exit cleanly with no tier appended.
- ~~Candidates JSON persistence~~ → nuke after Phase 2 (commit or
  zero-survivor exit). Roadmap is sole source of truth.
- ~~Modify `/audit-all`~~ → out of scope here; the audit-all post-run
  workflow (Claude appends findings to roadmap, then nukes JSONL
  artifacts) is a separate convention captured in MEMORY.md.

## Implementation note

Each skill is a single `SKILL.md` file installed at
`~/.claude/skills/<name>/SKILL.md`. Mirrors the shape of
`audit-all/SKILL.md` and `roadmap-pickup/SKILL.md`. No additional
files, configs, or shared modules are required — the staging JSON
convention is documented in each `SKILL.md` and lives in
`.claude/audits/` per okdctl convention.
