# Contributing to okdctl

Thanks for your interest in contributing. This document covers the
practical gates a change has to clear — dev setup, local checks, and the
conventions CI enforces.

## Development setup

You need Go 1.26 or newer (`go.mod` pins the toolchain, so `go build`
will fetch the right one automatically).

```sh
git clone https://github.com/qxtaiba/okdctl.git
cd okdctl
make build        # builds ./bin/okdctl
make test         # unit tests with -race and coverage
make lint         # golangci-lint (installed on first run)
```

Install [lefthook](https://github.com/evilmartians/lefthook) and enable
the repo's git hooks — they run gofumpt, `go vet`, and shellcheck on
commit, and build + test on push:

```sh
brew install lefthook   # or: go install github.com/evilmartians/lefthook/v2@v2.1.10
lefthook install
```

Development on macOS works fine for building and testing, but the
deploy path (package installs, `firewall-cmd`, `nmcli`, `systemctl`) is
Linux-only and much of it lives behind `//go:build linux`. An actual
deploy needs a Linux host; macOS lint runs also skip linux-tagged files,
so expect CI to catch issues there.

## Before you submit

Run `make test && make lint` and make sure both pass. CI runs the same
checks plus a few more, and everything must be green before merge:

- **Tests** run with `-race`. The `test-go` job also enforces
  per-package coverage floors from `.github/coverage-floors.conf` — if
  your change drops a listed package below its floor, CI fails. When
  you add tests that raise a package's coverage, raise its floor too.
- **Lint** config lives in `.golangci.yml`. The thresholds you are most
  likely to hit: functions max 120 lines (`funlen`), cyclomatic
  complexity max 30 (`gocyclo`), duplicate-code threshold 100 tokens
  (`dupl`).
- **YAML** is checked with `yamlfmt -lint .`.
- **Terraform** under `infrastructure/terraform/` must pass
  `terraform fmt -check`, `terraform validate`, and tflint.

## Commit messages

Conventional commits: `type(scope): description` — lowercase,
imperative, subject under 70 characters. Types in use: `feat`, `fix`,
`refactor`, `chore`, `ci`, `docs`, `test`. Use the optional body for the
*why* and non-obvious trade-offs; the diff already shows the what. See
`git log --oneline` for examples.

## User-facing changes

- Add a `CHANGELOG.md` entry under `[Unreleased]` for anything a user
  would notice (new flags, changed output, fixed bugs). Skip it for
  refactors and CI-only changes.
- If you add or change CLI commands or flags, run `make docs` and commit
  the regenerated pages under `docs/cli/` — CI fails on drift.

## Flag conventions

- `--output`/`-o` selects the output *format* (`text`, `json`),
  mirroring kubectl/oc. Never bind `-o` to anything else; a flag that
  writes to a file path is `--output-file` with no shorthand.
- Single-letter shorthands are an allowlist: `--yes`/`-y`,
  `--quiet`/`-q`, `--verbose`/`-v`, `--output`/`-o`, `--config`/`-c`.
- Per-command boolean tail flags (`--skip-must-gather`, `--dry-run`,
  `--keep-isos`, ...) are long-form only. Don't add shorthands to new
  ones.

## Finding your way around

Architecture docs live in `docs/architecture/` — start with
`phases.md` for the deploy phase model, `wizard.md` for the interactive
setup flow, and `addons.md` for the addon system. The generated CLI
reference is under `docs/cli/`.

## Reporting bugs

Use the issue forms — they ask for exactly what's needed to reproduce
(`okdctl version`, `okdctl doctor` output, and a `okdctl debug-bundle`
attachment). Security vulnerabilities go through GitHub Security
Advisories, not public issues — see [SECURITY.md](SECURITY.md).
