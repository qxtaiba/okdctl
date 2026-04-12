# Repo conventions for AI contributors

This file is the durable instruction set for Claude, GPT-based tools, and any
other agent that edits this repository. The rules here are authoritative —
they override defaults baked into the assistant. If a section is missing,
fall back to standard Go community conventions (gofmt, effective-go, the
golangci-lint config in `.golangci.yml`).

## Commit messages

- **Format**: `type(scope): description` (conventional commits)
- **Style**: lowercase, imperative, short (under 70 chars for the subject line)
- **Types in use**: `refactor`, `fix`, `feat`, `chore`, `ci`, `docs`, `test`
- **No trailers.** Do not add `Co-Authored-By: Claude <...>` or similar. Do not
  reference AI, LLMs, or any tool branding in commit bodies.
- **Body** (optional): describe the *why* and any non-obvious trade-offs, not
  the what (the diff already shows that).

See `git log --oneline` for existing examples.

## Code comments

This repo targets ~3% comment density. Go's naming conventions and the type
system already carry most of the signal; comments should add what the code
can't say for itself.

### Write a comment when (and only when)

1. **Package doc**. Every package has a `// Package X ...` block above the
   package clause, one to three sentences.
2. **Exported API with non-obvious behavior**: ordering constraints, caller
   contracts, failure modes, or mutation semantics that aren't evident from
   the signature.
3. **Non-obvious WHY decisions**: a workaround, a security-relevant choice, a
   performance decision that beats the "obvious" version, a ctx/cancellation
   subtlety, a lint suppression.
4. **Workarounds with context**: link the upstream bug or issue when you're
   working around it. Example: `// charmbracelet/bubbles#812 — TextStyle
   breaks width calc`.
5. **Subtle invariants and footguns**: "caller must hold mu", "not safe after
   Close()", "never log raw password — scrub via X".

### Do NOT write a comment when

1. **The name already says it.** Bad:
   ```go
   Key string      // unique key
   Label string    // display label
   Required bool   // whether the field is required
   ```
   These add zero signal. Delete.

2. **The function doc just echoes the signature.** Bad:
   ```go
   // GetFoo returns foo.
   func (x *X) GetFoo() Foo { return x.foo }
   ```
   Either add semantic info or delete the comment.

3. **You're narrating your own refactor.** Bad:
   ```go
   // Extracted from X because the body was 40 lines.
   // Split into 4 sub-methods to stay under funlen.
   ```
   The commit message already carries this. Delete.

4. **Self-referential peacock talk.** Bad:
   ```go
   // This is the canonical "does X exist?" check.
   // Use this function for ...
   // This helper function ...
   ```
   Replace with a plain one-line description or delete.

5. **Section dividers.** Bad:
   ```go
   // --- Auth ---
   // === helpers ===
   ```
   Use file splits or function names. Delete dividers.

6. **Inline comments that narrate the next line.** Bad:
   ```go
   // Create the step builder.
   b := distribution.NewStepBuilder(id, name)
   ```
   If the code isn't self-evident, rename variables.

7. **TODO / FIXME without context.** If you must leave one, name the person
   and link an issue: `// TODO(@handle): see owner/repo#123`. Bare `// TODO`
   is rot.

### Comment format

- Use `//` line comments, not `/* ... */` blocks, except for build tags or
  embed directives.
- Start doc comments with the identifier name: `// Foo does X.`
- Keep doc comments to 1–3 sentences unless there's a real invariant to
  explain. If you're writing a paragraph, ask whether the function is too
  complex or the doc is over-explaining.
- Wrap lines at ~80 columns for readability, ~100 max.
- No trailing periods on field-level inline comments (`Foo int // count`), do
  use them in full-sentence doc comments.

### When in doubt

Delete the comment. If the code breaks later, the commit history and the
type system will catch it. If a reviewer asks "why?", that's the signal to
write the comment — then it carries real information.

## Architecture notes

- `internal/distribution/okd/phase/` holds shared helpers used by all OKD
  phases (`setup`, `install`, `postinstall`, `destroy`, `cleanup`). New
  cross-phase helpers belong here, not in a specific phase package.
- `internal/distribution.StepDef` + `BuildSteps` is the canonical way to
  declare phase steps. Don't introduce new per-step builder functions;
  add entries to the phase's existing `xSteps()` method.
- `internal/logutil.NopLogger` is the canonical no-op logger. Don't
  write `slog.New(slog.DiscardHandler)` inline.
- `internal/system.WriteTempFile` handles temp-file-with-callback.
  Don't hand-roll `os.CreateTemp` + chmod + defer cleanup.
- `BasePhase.OcResourceExists` and `BasePhase.OcPollOutput` are the canonical
  kubectl/oc helpers for phase code. Extend `phase/kubectl.go` rather than
  writing local wrappers.
- `addon.BuildOpaqueSecret` is the canonical k8s Opaque Secret manifest
  builder for addons.

## Tooling

- `go.mod` targets Go 1.25 / toolchain 1.26.2. Don't downgrade.
- `.golangci.yml` is authoritative for lint config. Key thresholds:
  `funlen.lines: 120`, `gocyclo.min-complexity: 30`, `dupl.threshold: 200`.
- CI runs `lint-go`, `test-go`, `build-go`, `security` (govulncheck),
  `lint-yaml`, `validate-terraform`. All must be green before merging to
  `develop`.
- Pre-commit hooks run via `lefthook`.

## Credentials and secrets

- Never commit secrets. `.env` files live alongside `configs/*.yaml` and are
  gitignored.
- Password fields in `credentials.ProxmoxCredentials` use `[]byte` so callers
  can `Zeroize` them after use.
- Error messages must not leak raw credentials. See `InputField.Validate`
  for the password scrubbing pattern.
