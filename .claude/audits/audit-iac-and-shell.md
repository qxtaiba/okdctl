# audit-iac-and-shell — 2026-05-08

**Assumes green:** golangci-lint, govulncheck, CodeQL, shellcheck (clean
on `scripts/install.sh`), `terraform fmt -check -recursive` (clean),
tflint with `terraform/recommended` preset (CI), tfsec (CI, soft-fail),
go test ./...
**Scope:** `scripts/install.sh` and `infrastructure/terraform/**/*.tf`
(modules + environments/production), plus the IaC-relevant slice of
`.github/workflows/ci.yml` (tflint/tfsec wiring).
**Out of scope this run:** `internal/distribution/okd/templates/terraform.tfvars.tmpl`
is rendered by Go, so credential plumbing through it belongs to
audit-security per seam §6. Provider auth via env vars (PROXMOX_VE_*)
is also Go-side plumbing.
**Seam co-owners:**
- audit-security — install.sh trust-anchor *policy* (this audit catalogs
  the partial-trust site at L141-L145; security frames whether the
  policy is acceptable). PROXMOX_VE_* env-var consumption from Go.
- audit-state-and-recovery — Ceph data-disk destruction recoverability
  (this audit flags the IaC hygiene gap; state-and-recovery owns the
  partial-failure recovery story).

## Executive summary

The IaC surface is small (4 module .tf files + 4 env .tf files) and
shellcheck-clean. The biggest cluster is **install-sh-integrity** —
the cosign-absent branch silently downgrades to sha256-only trust against
a release origin that signs *both* the binary and SHA256SUMS, which CWE-494
treats as no integrity check at all. Secondary cluster is **HCL variable
semantics** — three "defaults to X" variables in `variables.tf` carry
non-null defaults that nullify the documented coalesce-fallback. Two CI
config drifts (tfsec `soft_fail: true`, tflint without a version pin)
land in linter-config-bug territory. No secrets in HCL locals; no
provider credentials hardcoded; destroy ordering is correct for compute
nodes. Total findings: 8 (blocker 0, major 2, minor 5, suggestion 1).

## Ranked table

| ID | Cluster | File:line | Severity | Confidence | LOC | Adjacent linter | Fix class |
|----|---------|-----------|----------|------------|-----|-----------------|-----------|
| `iac:e076e43c:cosign-optional-when-absent` | install-sh-integrity | scripts/install.sh:141-146 | major | high | ~6 | shellcheck:none | policy |
| `iac:b803fcb7:tfsec-soft-fail` | hcl-provider-hygiene | .github/workflows/ci.yml:114-117 | major | high | 1 | tflint:none | config |
| `iac:ef8f2924:bootstrap-default-shadows-coalesce` | hcl-credential-hygiene | infrastructure/terraform/modules/proxmox-okd/variables.tf:200-228 | minor | high | ~10 | tflint:none | refactor |
| `iac:e076e43c:tar-zipslip-incomplete` | install-sh-integrity | scripts/install.sh:165-167 | minor | medium | 1 | shellcheck:none | refactor |
| `iac:b803fcb7:tflint-version-unpinned` | hcl-provider-hygiene | .github/workflows/ci.yml:101 | minor | high | 2 | none | config |
| `iac:18a795d5:depends-on-bootstrap-artificial` | hcl-destroy-ordering | infrastructure/terraform/modules/proxmox-okd/main.tf:243 | minor | medium | 1 | tflint:none | refactor |
| `iac:18a795d5:worker-data-disk-no-prevent-destroy` | hcl-destroy-ordering | infrastructure/terraform/modules/proxmox-okd/main.tf:328-339 | minor | medium | ~12 | tflint:none | refactor |
| `iac:04b033b9:provider-insecure-not-pinned-false` | hcl-credential-hygiene | infrastructure/terraform/environments/production/versions.tf:1-10 | suggestion | medium | ~5 | tflint:none | refactor |

## Findings

### `iac:e076e43c:cosign-optional-when-absent`

**ID:** `iac:e076e43c:cosign-optional-when-absent`
**Cluster:** install-sh-integrity
**File + line range:** `scripts/install.sh:141-146`
**Current LOC touched:** 6
**Smell:** When `cosign` is not installed and `INSECURE` is unset, the
script silently downgrades to sha256-only trust. SHA256SUMS is fetched
from the same `releases/download/$VERSION` URL as the binary, so a
single compromised release origin (or MITM that defeated TLS) replaces
both. CWE-494 calls this "Download of code without integrity check"
because sha256 from the same origin as the bytes verifies nothing the
TLS chain didn't already verify. Compare with the `INSECURE=1` branch
at L60-L66: if cosign IS installed, the user must explicitly opt in to
skip the check. The cosign-absent path bypasses that gate.
**Evidence:**
```
elif [ -n "$INSECURE" ]; then
    info "cosign signature verification skipped (INSECURE=1)"
else
    info "cosign not installed — skipping signature verification"
    info "install cosign from https://docs.sigstore.dev/system_config/installation/ to enable signature verification"
fi
```
**Fix — preferred:** policy. Either (a) require cosign as a hard
prerequisite (mirror the `sha256sum` enforcement at L49-L53 — refuse
install when absent, with the same install-with-your-package-manager
hint), or (b) print the message at WARN level via `red()` (matches the
`INSECURE=1` branch styling at L64-L65) so the user sees a visible
trust-degradation rather than an info-level "skipping". Most ecosystems
(helm, kind, terraform) take path (a) on install.sh today.
**Rule source:** CWE-494 (Download of code without integrity check),
repo-counter-example: `scripts/install.sh:L49-L53` (sha256sum is required,
not optional)
**Adjacent linter:** none (shellcheck doesn't model trust policy)
**Scaffolding?:** no
**Seam:** audit-security (security owns the *policy* — whether
shipping with optional-cosign is acceptable for okdctl's threat model;
this audit catalogs the *site*)
**What MUST stay bit-for-bit:** the `INSECURE=1` enforcement at L60-L66
that refuses to bypass when cosign is unavailable; the `set -euo pipefail`
discipline; the `curl_safe` hardened wrapper.
**Estimated net LOC delta:** +3 (turn the else branch into a `die`
+ install-cosign hint)
**Severity:** major
**Risk (of applying fix):** medium — making cosign mandatory breaks
distros that ship it via a non-default channel; users on minimal images
need a remediation path.
**Confidence (in finding):** high — the file's own enforcement at
L49-L53 (sha256sum) and L60-L66 (INSECURE=1) shows the project knows
this pattern; cosign falls outside it without justification.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### `iac:b803fcb7:tfsec-soft-fail`

**ID:** `iac:b803fcb7:tfsec-soft-fail`
**Cluster:** hcl-provider-hygiene
**File + line range:** `.github/workflows/ci.yml:114-117`
**Current LOC touched:** 1
**Smell:** `tfsec-action` runs with `soft_fail: true`, so HCL security
findings produce job logs but never fail the `scan-terraform` job.
CLAUDE.md `§Tooling` states "All must be green before merging to
`develop`" but `soft_fail` makes tfsec un-gating — the green check is
cosmetic. Either make it gating, or delete the job (a passing job
nobody reads is worse than no job).
**Evidence:**
```yaml
      - uses: aquasecurity/tfsec-action@b466648d6e39e7c75324f25d83891162a721f2d6 # v1.0.3
        with:
          working_directory: infrastructure/terraform
          soft_fail: true
```
**Fix — preferred:** config. Drop `soft_fail: true` (or set it to
`false`); add a `.tfsec/config.yml` to suppress known-acceptable
findings by rule code if any exist. Alternatively, list in
SKIP/IGNORE list with documented justification per rule.
**Rule source:** CLAUDE.md `§Tooling` ("CI runs ... All must be green
before merging to `develop`")
**Adjacent linter:** none (tfsec IS the linter, the question is whether
its result gates)
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the SHA-pinned action ref `# v1.0.3`
(CLAUDE.md `§Pin stability`).
**Estimated net LOC delta:** -1 (delete `soft_fail: true`)
**Severity:** major
**Severity reason:** rubric §4/un-idiomatic-pattern that has bitten
the repo before — non-gating CI is a known regression channel.
**Risk (of applying fix):** medium — turning on may surface dormant
findings; expect a one-time triage pass.
**Confidence (in finding):** high — `soft_fail` is the documented
non-gating mode in tfsec-action; intent is unambiguous.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### `iac:ef8f2924:bootstrap-default-shadows-coalesce`

**ID:** `iac:ef8f2924:bootstrap-default-shadows-coalesce`
**Cluster:** hcl-credential-hygiene (variable-semantics sub-category;
no credential implication, but the cluster catches "variables that
don't do what their description says")
**File + line range:** `infrastructure/terraform/modules/proxmox-okd/variables.tf:200-228`
**Current LOC touched:** 10
**Smell:** Three variables described as "defaults to X if not set"
carry non-null defaults that defeat the coalesce-fallback in
`main.tf::locals`:
- `bootstrap_memory_mb` default = `8192` (L203). `coalesce(8192, var.memory_mb)` always returns 8192 — setting `memory_mb=16384` does not raise bootstrap memory.
- `worker_cpu_cores` default = `8` (L221). Same shadowing.
- `worker_memory_mb` default = `20480` (L227). Same shadowing.
Compare with `master_cpu_cores`, `master_memory_mb`, `bootstrap_cpu_cores`
which correctly default to `null` and DO fall through. The asymmetry
silently overrides operator-set `memory_mb` / `cpu_cores` for these
three node-roles.
**Evidence:**
```hcl
variable "bootstrap_memory_mb" {
  description = "memory for bootstrap node (defaults to memory_mb if not set)"
  type        = number
  default     = 8192   # ← non-null defeats coalesce(var.bootstrap_memory_mb, var.memory_mb)
}
```
**Fix — preferred:** refactor. Set defaults to `null` to match the
documented "fallback to memory_mb / cpu_cores" semantics. If the
intent is a per-role floor (e.g., bootstrap should be at least 8 GiB
even if `memory_mb` is lower), encode the floor via `validation`
or `max(local.cluster_memory, 8192)` rather than via a default that
shadows the input.
**Rule source:** repo-counter-example:
`infrastructure/terraform/modules/proxmox-okd/variables.tf:194-216`
(`bootstrap_cpu_cores`, `master_cpu_cores`, `master_memory_mb` all
correctly use `default = null` for the same fallback pattern)
**Adjacent linter:** none (HCL semantics; tflint doesn't model
coalesce-vs-default intent)
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the validation blocks at L40-L43,
L50-L53, L60-L63, etc.; the coalesce expressions in `main.tf::locals`.
**Estimated net LOC delta:** 0 (just flip three default values)
**Severity:** minor
**Risk (of applying fix):** low — semantics align with description;
existing operators with explicit values are unaffected.
**Confidence (in finding):** high — `coalesce(non_null, x)` always
returns the first arg; this is HCL spec.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### `iac:e076e43c:tar-zipslip-incomplete`

**ID:** `iac:e076e43c:tar-zipslip-incomplete`
**Cluster:** install-sh-integrity
**File + line range:** `scripts/install.sh:165-167`
**Current LOC touched:** 1
**Smell:** The defense-in-depth zip-slip check uses `grep -qE
'^(\.\.|/)'` which only catches entries STARTING with `..` or `/`.
A tar entry like `subdir/../../etc/passwd` starts with `subdir/` and
silently passes. Goreleaser tarballs are flat (the comment on L165-L166
acknowledges this) and the cosign+sha256 chain is the primary guard,
so this is purely defense-in-depth — but as written it doesn't
defend against the canonical tar zip-slip pattern.
**Evidence:**
```bash
tar -tzf "$ARCHIVE_NAME" | grep -qE '^(\.\.|/)' && die "archive contains absolute or parent-traversal paths"
```
**Fix — preferred:** refactor. Match the canonical anti-zip-slip
pattern: `grep -qE '(^|/)\.\.(/|$)|^/'` — catches `..` as the first
segment, any internal `/../` segment, or any absolute path. Or use
`tar --no-anchored --exclude='*/../*' --exclude='/*'` to fail at
extraction time without parsing the listing.
**Rule source:** CWE-22 (Path Traversal), Snyk "Zip Slip" advisory
catalog (which canonicalizes the `(^|/)\.\.(/|$)` regex).
**Adjacent linter:** none (shellcheck doesn't pattern-match regexes)
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the `tar --no-same-owner
--no-same-permissions --no-overwrite-dir` flags on the actual extract
at L168 (those ARE primary defenses against owner/perm tampering).
**Estimated net LOC delta:** 0
**Severity:** minor
**Risk (of applying fix):** low — strictly more conservative regex.
**Confidence (in finding):** medium — the comment explicitly says
"defense-in-depth", so the project's intent is to layer protection;
the current regex doesn't deliver the layer it claims to.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### `iac:b803fcb7:tflint-version-unpinned`

**ID:** `iac:b803fcb7:tflint-version-unpinned`
**Cluster:** hcl-provider-hygiene
**File + line range:** `.github/workflows/ci.yml:101`
**Current LOC touched:** 2
**Smell:** `terraform-linters/setup-tflint@<sha> # v6.2.2` is correctly
SHA-pinned per CLAUDE.md `§Pin stability`, but the action's `with:`
block omits `tflint_version`, so it installs whichever tflint version
the action's default points at on the runner. CLAUDE.md states "Tool
installs from Go must be explicit versions — never `@latest`." The
same logic applies to tflint, which is also a Go-installed binary.
Compare with `terraform_version: "1.10.3"` at L91, which IS patch-pinned.
**Evidence:**
```yaml
      - uses: terraform-linters/setup-tflint@b480b8fcdaa6f2c577f8e4fa799e89e756bb7c93 # v6.2.2
      - working-directory: infrastructure/terraform/modules/proxmox-okd
        run: |
          tflint --init --config="${GITHUB_WORKSPACE}/infrastructure/terraform/.tflint.hcl"
```
**Fix — preferred:** config. Add `with: { tflint_version: "v0.55.0" }`
(or current pin) to both `setup-tflint` invocations. Rev together with
ruleset bumps.
**Rule source:** CLAUDE.md `§Dependencies / Pin stability` ("Tool
installs from Go must be explicit versions — never `@latest`. Terraform
versions in CI are patch-pinned.")
**Adjacent linter:** none (no linter checks GHA action input args
against repo policy)
**Scaffolding?:** no
**Seam:** audit-dependencies (dep-pin policy umbrella; this audit
catalogs the IaC site)
**What MUST stay bit-for-bit:** the SHA pin on the action ref.
**Estimated net LOC delta:** +2 (one with-block per `setup-tflint`)
**Severity:** minor
**Risk (of applying fix):** low.
**Confidence (in finding):** high — the action's input is documented;
omission means default-track-latest.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### `iac:18a795d5:depends-on-bootstrap-artificial`

**ID:** `iac:18a795d5:depends-on-bootstrap-artificial`
**Cluster:** hcl-destroy-ordering
**File + line range:** `infrastructure/terraform/modules/proxmox-okd/main.tf:243`
**Current LOC touched:** 1
**Smell:** `proxmox_virtual_environment_vm.master` declares
`depends_on = [proxmox_virtual_environment_vm.bootstrap]`. There is no
attribute reference from master to bootstrap, and at the Proxmox API
level the resources are independent (no shared CDROM or volume) —
masters do not need bootstrap to exist before they're created at the
infrastructure layer. The OKD-installer-level dependency (masters need
the bootstrap node to come up so they can fetch ignition / pivot) is
NOT something Terraform can or should manage; that ordering lives in
`internal/distribution/okd/install/`. Same pattern at L380 for
worker → master. Hashicorp's own guidance is to use `depends_on` ONLY
when the dependency cannot be expressed via attribute reference; here,
no real Terraform-graph dependency exists, so this is overspecification.
On destroy, this forces workers → masters → bootstrap order, which
is desirable; on apply with `bootstrap_enabled=false`, it makes the
plan more brittle than necessary.
**Evidence:**
```hcl
# resource "proxmox_virtual_environment_vm" "master" { ...
  depends_on = [proxmox_virtual_environment_vm.bootstrap]
# }
```
**Fix — preferred:** refactor. Drop both `depends_on`. If destroy
ordering across roles matters (it might for Proxmox storage release
or VMID overlap during quick cycle), document the operational
sequence in the README destroy section instead — or rely on the
explicit two-phase destroy in `internal/distribution/okd/destroy/`.
**Rule source:** Terraform docs ("Use `depends_on` as a last resort,
when you have no other way to convey the dependency"); repo
counter-example: bootstrap, master, worker resources have no
attribute-level interdependence, so the explicit `depends_on` is the
ONLY graph edge — meaning it's a manual-only edge.
**Adjacent linter:** tflint:none (tflint's terraform/recommended
doesn't catch overspecified `depends_on`)
**Scaffolding?:** no
**Seam:** audit-state-and-recovery (recovery owns the destroy
sequence; this audit catalogs the IaC-graph hygiene)
**What MUST stay bit-for-bit:** the `prevent_destroy = true` on
master at L256; the `lifecycle.precondition` blocks at L122-L137 /
L257-L260 / L383-L386.
**Estimated net LOC delta:** -2
**Severity:** minor
**Risk (of applying fix):** medium — if some downstream operational
sequence depends on this Terraform-level ordering (e.g., quick
recreate where bootstrap CDROM is freed before master mounts it),
removing the edge could surface a race. Verify with a destroy/apply
cycle in dev.
**Confidence (in finding):** medium — overspecified `depends_on` is
a known anti-pattern, but the okdctl-specific deploy sequence may
have a reason this audit doesn't see.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### `iac:18a795d5:worker-data-disk-no-prevent-destroy`

**ID:** `iac:18a795d5:worker-data-disk-no-prevent-destroy`
**Cluster:** hcl-destroy-ordering
**File + line range:** `infrastructure/terraform/modules/proxmox-okd/main.tf:328-339`
**Current LOC touched:** 12
**Smell:** Workers attach a 500 GiB Ceph data disk by default
(`worker_data_disk_size_gb = 500` per `variables.tf:85`) under a
`dynamic "disk"` block. The variable's description warns that
"lowering this after initial apply destroys the ceph data disk", but
the resource has no `prevent_destroy = true` (unlike masters), and the
`dynamic "disk"` is NOT in `lifecycle.ignore_changes` — only
`network_device, startup, cdrom, boot_order, efi_disk` are. A
`terraform apply` with `worker_data_disk_size_gb = 0` (or anything
below `minimum_data_disk_size_gb`) silently destroys the data disk.
The variable description warns; the IaC layer doesn't.
**Evidence:**
```hcl
  dynamic "disk" {
    for_each = var.worker_data_disk_size_gb >= var.minimum_data_disk_size_gb && var.worker_data_disk_size_gb > 0 ? [1] : []
    content {
      datastore_id = var.data_storage
      size         = var.worker_data_disk_size_gb
      ...
      serial       = "CEPH-DATA"
    }
  }
  ...
  lifecycle {
    ignore_changes = [
      network_device, startup, cdrom, boot_order, efi_disk,
      # disk NOT listed
    ]
  }
```
**Fix — preferred:** refactor. Either (a) add `prevent_destroy = true`
on the worker resource for production-mode (matching the master
pattern with the documented `override.tf` escape hatch), or (b) extend
`lifecycle.ignore_changes` to include `disk` for the data-disk
attributes, so subsequent applies do not size-down. Option (b) is
narrower; option (a) gives operator-grade protection at the cost of
the override-file procedure.
**Rule source:** rubric §4/data-loss anchor (Ceph data destruction is
data-loss); repo-counter-example: master `prevent_destroy = true` at
`main.tf:256` (same protection pattern, applied where the data
sensitivity is comparable).
**Adjacent linter:** tflint:none (no rule for "data-bearing resource
without prevent_destroy")
**Scaffolding?:** no
**Seam:** audit-state-and-recovery (recovery owns the partial-failure
story; this audit catalogs the IaC hygiene gap)
**What MUST stay bit-for-bit:** the `dynamic "disk"` shape; the
`minimum_data_disk_size_gb` floor mechanism described at
`variables.tf:73`.
**Estimated net LOC delta:** +2
**Severity:** minor
**Severity reason:** documented in variable description and gated by
`minimum_data_disk_size_gb` floor — not silent — but operator-grade
protection still belongs in the lifecycle block.
**Risk (of applying fix):** medium — adds friction to legitimate
worker re-sizing; the master comment at `main.tf:245-254` documents
the override.tf escape hatch.
**Confidence (in finding):** medium — design choice; documented
behavior diverges from master treatment of similar data sensitivity.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### `iac:04b033b9:provider-insecure-not-pinned-false`

**ID:** `iac:04b033b9:provider-insecure-not-pinned-false`
**Cluster:** hcl-credential-hygiene
**File + line range:** `infrastructure/terraform/environments/production/versions.tf:1-10`
**Current LOC touched:** 5 (would be additive)
**Smell:** Production env declares the `bpg/proxmox` provider via
`required_providers` but has no `provider "proxmox" {}` block. The
provider therefore consumes `PROXMOX_VE_INSECURE` from the environment.
Comments in `variables.tf:7` and `main.tf:10` say "DEV ONLY: never set
in prod" — but production HCL doesn't enforce that. An operator who
exports `PROXMOX_VE_INSECURE=true` for a quick test and re-runs
`okdctl deploy` against production will silently disable TLS
verification with no audit trail in the .tf files.
**Evidence:**
```hcl
terraform {
  required_version = ">= 1.10, < 2.0"
  required_providers {
    proxmox = {
      source  = "bpg/proxmox"
      version = "~> 0.106.0"
    }
  }
}
# no provider "proxmox" {} block — env-var path is the only configuration
```
**Fix — preferred:** refactor. Add an explicit `provider "proxmox" {}`
block in production env that pins `insecure = false` (and lets endpoint
/ username / password come from env). Dev environments (when added)
keep the unset block. This makes the security stance auditable in HCL.
**Rule source:** CLAUDE.md `§Credentials and secrets`, security
invariants §13 ("TLS verification — never recommend `InsecureSkipVerify`")
**Adjacent linter:** tflint:none (no rule for "production env should
hard-pin insecure=false")
**Scaffolding?:** no
**Seam:** audit-security (security owns the TLS policy framing; this
audit catalogs the HCL hygiene gap)
**What MUST stay bit-for-bit:** the env-var path for endpoint / username
/ password — those legitimately stay in the environment, not HCL.
**Estimated net LOC delta:** +4
**Severity:** suggestion
**Risk (of applying fix):** low — a hard `insecure = false` is the
documented production stance.
**Confidence (in finding):** medium — depends on operator threat
model; many shops accept env-var TLS toggles.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

## Scaffolding items detected

None this run. The `protect_masters` variable in
`modules/proxmox-okd/variables.tf:328-332` carries
`tflint-ignore: terraform_unused_declarations` with a justification
comment — that is the documented intent flag pattern, not scaffolding
per MEMORY.md §scaffolding.

## Linter-config-bug candidates

- `iac:b803fcb7:tfsec-soft-fail` — tfsec IS wired in CI but configured
  non-gating. This is the `.golangci.yml`-equivalent config bug for
  the IaC tier.
- `iac:b803fcb7:tflint-version-unpinned` — tflint IS wired but the
  binary version isn't pinned. Sister-config-bug in the same
  `validate-terraform` job.

## Skip list

- **install.sh `head -1` instead of `head -n 1` (L97)** — POSIX
  prefers `-n 1` but BSD/GNU both accept `-1`. shellcheck doesn't
  flag and the script's shebang is bash. No finding.
- **Module declares the `proxmox` provider via `required_providers`
  but no `provider "proxmox" {}` block exists** — bpg/proxmox supports
  env-var-only configuration; this is a valid pattern for a module
  that expects the consumer (env) to own the provider block. tflint's
  recommended preset accepts it. The TLS-pinning concern is captured
  in `iac:04b033b9:provider-insecure-not-pinned-false`.
- **Bootstrap and worker have no `prevent_destroy = true`** — bootstrap
  is by-design ephemeral; worker carries the data-disk concern captured
  separately as `iac:18a795d5:worker-data-disk-no-prevent-destroy`.
- **`protect_masters` variable declared but unused** — has a
  `tflint-ignore` comment + justification block; documented intent flag.
- **`agent { enabled = false }`** — design choice (faster Terraform
  ops, avoids qemu-guest-agent ipv6/ipv4 races); not an audit finding.
- **Three duplicated `# - PROXMOX_VE_INSECURE` lines in
  `variables.tf:6-7`, `main.tf:9-10`, env `variables.tf:6-7`** — comment
  density redundancy; audit-documentation territory, not iac-and-shell.

## Cluster verdicts

**install-sh-fail-closed:** Clean. `set -euo pipefail`, EXIT/INT/TERM
trap, `mktemp -d`, `curl --fail` via `-sSfL` shorthand, all tempfile
writes go to `$TMP` which is cleaned up. No `rm -rf $var` patterns. No
shellcheck output.

**install-sh-integrity:** One major (`cosign-optional-when-absent`)
plus one minor (`tar-zipslip-incomplete`). The sha256 verification
chain is correct (awk field-equality, `$EXPECTED` checked non-empty,
constant-time-ish equality). The cosign chain when present is correct
(certificate-identity-regexp, oidc-issuer pinned). The hole is the
"cosign absent" silent fallthrough.

**hcl-credential-hygiene:** Clean for the usual smells — no secrets in
`locals`, no hardcoded credentials in any provider block (no provider
block exists anywhere), no `output` exposes a secret, all `sensitive=true`
applications would be over-marking since no value carries a secret.
The variable-semantics issue (`bootstrap-default-shadows-coalesce`)
lives in this cluster as a "what the variable claims to do vs what it
does" hygiene gap. The TLS-pin-false suggestion lives here.

**hcl-destroy-ordering:** Mostly correct. Compute order via `depends_on`
(workers → masters → bootstrap) is reversed correctly on destroy.
Master `prevent_destroy = true` is the right default; the documented
override.tf procedure at `main.tf:245-254` is the right escape hatch.
Two minors: artificial `depends_on` and the worker data-disk
prevention gap.

**hcl-provider-hygiene:** Versions block is patch-conservative
(`>= 1.10, < 2.0` for terraform, `~> 0.106.0` for bpg/proxmox).
required_providers is present in BOTH module versions.tf and env
versions.tf — that's correct. The two majors live in CI: tfsec
soft-failing, tflint version-unpinned.

## Scope exceptions proposed

None. The `terraform.tfvars.tmpl` Go template (rendered at deploy time)
is rightly out of scope per seam §6 — Go-side credential plumbing is
audit-security's surface.

## Footer

Total findings: 8 (blocker: 0, major: 2, minor: 5, suggestion: 1)
Scope coverage: 8 / 8 in-scope files read in full (0% via sub-agent
dispatch — surface is small enough for direct read).
Seam deferrals: 4 (`iac:e076e43c:cosign-optional-when-absent` →
audit-security; `iac:b803fcb7:tflint-version-unpinned` →
audit-dependencies; `iac:18a795d5:depends-on-bootstrap-artificial` and
`iac:18a795d5:worker-data-disk-no-prevent-destroy` →
audit-state-and-recovery).
shellcheck output: clean (exit 0).
terraform fmt -check: clean (exit 0).
To refresh `linter-config-bugs.jsonl`, run the aggregation command or
`/audit-all`.
