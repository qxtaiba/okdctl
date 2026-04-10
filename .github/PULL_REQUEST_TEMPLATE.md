<!--
Thanks for contributing! A small checklist before submitting to save
reviewer time. Not every PR needs every box checked — use your judgment.
-->

## Summary

<!-- One or two sentences: what changed and why. The diff shows WHAT; this section should explain WHY. -->

## Related issue

<!-- Fixes #123 / Closes #123 / Addresses #123. If none, briefly explain the motivation. -->

## Type of change

- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that changes existing behavior in a
      way users need to adapt to)
- [ ] Documentation only
- [ ] Refactor (no behavior change)
- [ ] Build / CI / tooling

## Checklist

- [ ] `make test` passes locally
- [ ] `make lint` passes locally
- [ ] `make compat-check` passes (if you edited the compat matrix)
- [ ] Tests added or updated (or explained why not)
- [ ] `CHANGELOG.md` updated under `[Unreleased]` with a user-visible entry
      (skip for refactor / CI / docs-only)
- [ ] Documentation updated if behavior or flags changed
- [ ] Commit messages follow `type(scope): description` (conventional
      commits, lowercase, imperative, under 70 chars)

## How I tested

<!--
For code changes, describe how you verified the change works. For a
bug fix, explain how you reproduced the bug and confirmed the fix.
"Manually tested on my Proxmox 8.2 cluster with OKD 4.16" is enough.
-->

## Breaking changes

<!--
If this is a breaking change, describe the migration path users will
need to follow. If it changes the on-disk state format or the config
file schema, note that prominently.
-->
