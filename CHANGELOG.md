# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `okdctl doctor` — preflight checks for OS, running as root, required
  binaries and packages, bin dir writability and presence on `$PATH`,
  passwordless sudo, SSH public key, pull-secret validity, disk space,
  and free ports
- `okdctl debug-bundle` — collects logs, deploy state, and diagnostics
  into a size-capped, credential-redacted support tarball
- global `--log-level`, `--log-format`, and `--log-file` flags;
  `--log-format` defaults to `json` when stderr is not a TTY
- background update check on startup against the GitHub releases API
  (plain HTTPS GET, no identifiers, 24 h on-disk cache); disclosed in the
  README and root help, opt out with `OKDCTL_NO_UPDATE_CHECK=1`; the
  notice is skipped on dev builds and routed to stderr
- SSH host-key pinning for the Proxmox host:
  `provider.proxmox.ssh_host_fingerprint` matches a SHA256 fingerprint
  per key type, and `provider.proxmox.require_pinned_fingerprint: true`
  fails closed when no pin is configured; flux deploy-key git hosts get
  the same keyscan pinning
- HTTPS ignition serving with `coreos.inst.ca` certificate pinning;
  the ignition server IP must be an RFC1918 address
- `okdctl destroy --only={vms,workers,masters,bootstrap}` and
  `--target` for scoped destroys
- `--output`/`-o` (`text`|`json`) on `version`, `doctor`, `status`,
  `config show`, `describe`, `addon list`, `addon verify`, and
  `releases`
- `update-ingress --confirm-cluster` typo guard before mutating a live
  cluster
- deploy phase marker: a failed deploy now hints `cleanup` vs `destroy`
  based on the phase reached, and resume/destroy diagnostics read it back
- configurable binary install dir via config file and env override
- wizard accepts `worker_count: 0` for compact control-plane-only
  clusters
- a second Ctrl-C during graceful shutdown force-quits
- shell completion for flag values such as `releases --channel`
- terraform init retries on transient failures; state-locking commands
  pass `-lock-timeout=120s` and surface the stale lock id on contention
- Apache-2.0 license
- Architecture documentation under `docs/architecture/`
- Sigstore keyless signing on release artifacts
- SBOM (CycloneDX) attached to each release
- SLSA build provenance attestation
- `.deb` and `.rpm` packages published on release
- `curl | sh` installer script with SHA256 verification
- CodeQL SAST scan on every PR
- Community templates: issue forms (bug report, feature request),
  PR template, CODEOWNERS, and a CONTRIBUTING.md guide

### Changed
- **Breaking:** okdctl.yaml schema version bumped to `v2`. The control-plane
  node group is now named `control_plane` everywhere at the YAML surface, and
  every size key names its unit. Key renames (old → new):
  - `provider.proxmox.master_nodes` → `provider.proxmox.control_plane_nodes`
  - `disks.master_data_size_gb` → `disks.control_plane_data_size_gb`
  - `topology.<group>.memory` → `topology.<group>.memory_mb`
  - `topology.<group>.disk` → `topology.<group>.disk_gb`

  There is no automatic migration: a config with `schemaVersion: v1` fails to
  load with a message listing these renames; apply them and set
  `schemaVersion: "v2"`. Terraform variable names (`master_target_nodes`,
  `master_count`, ...) are unchanged — OKD's own master/worker vocabulary
  stays internal. Two validation tightenings ride along: placement lists
  longer than the group's topology count are rejected instead of silently
  truncated (shorter lists still pad with `provider.proxmox.node`), and a
  config that omits `schemaVersion` entirely is rejected (it previously
  inherited the default version silently).
- format-selector flags renamed `--format` → `--output`/`-o` on every
  command; file-destination flags renamed `--output` → `--output-file`
  (deploy, kubeconfig, debug-bundle) and no longer take `-o`
- the static IP plan is derived from `machine_cidr`: `static_ip.netmask`
  is computed at load time and a conflicting hand-set value is rejected;
  the `static_ip.start` default moved from `192.168.1.100` to
  `192.168.1.140` because the old default collided with a default
  Proxmox host, and a start equal to the Proxmox host or ignition
  server IP is now rejected
- exit codes follow BSD sysexits (usage 64, data 65, missing config 66,
  ...); `doctor` exits 2 when any check reports `[fail]`; full table in
  `docs/cli/exit-codes.md`
- `install.sh` fails closed: SHA256 verification is mandatory, the
  cosign signature check is required unless `INSECURE=1`, and malformed
  version tags abort
- JSON output modes suppress informational chatter so stdout stays
  machine-parseable; the update banner is silenced under `--quiet` and
  JSON output
- terraform protects master VMs with `prevent_destroy` and ignores
  master and worker data-disk changes so a plan can never recreate
  Ceph data disks
- ISO uploads run per-file so a resumed deploy only re-sends what is
  missing; ISO build/upload and ignition rendering skip work that is
  already done
- `doctor` compiles on every platform (running it still requires Linux)
- `describe addon --output=json` now emits `display_name` (snake_case) instead
  of `display-name`; aligns with all other JSON fields
- Release builds now use `-trimpath` for reproducibility
- Project renamed from `openshitctl` (binary) and `okd-proxmox-cli`
  (module path) to `okdctl` for both. Module path is now
  `github.com/qxtaiba/okdctl`. Anyone with the old paths needs to
  update their go.mod and re-clone the renamed GitHub repo.

### Fixed
- resuming a deploy after bootstrap no longer wipes live cluster state
  and credentials: the pre-deploy wipe is gated on the deploy-state
  marker and refuses when populated terraform state contradicts it
- `deploy` honors `--config`, and provider validation is hard-gated
  before any deploy work starts
- `destroy` exits non-zero when a teardown step failed, refuses corrupt
  terraform state instead of reporting a no-op success, and snapshots
  `terraform.tfstate` before running `terraform init`
- haproxy config backups use one naming scheme across setup,
  postinstall, and cleanup: rollback falls back to the pristine
  snapshot and cleanup no longer leaves root-owned backup residue
- `update-ingress` verifies the API over the VIP before removing
  haproxy and rolls DNS and the haproxy config back on failure
- `status` parses cluster operators from `oc -o json` instead of text
  columns, omits placeholder nodes, surfaces `oc` failures instead of
  swallowing them, and drops never-populated fields from the JSON schema
- credential hygiene: secrets are scrubbed from the startup argv log,
  subprocess stderr excerpts, and debug-bundle manifests; terraform
  executor env is zeroized at every phase site; kubeconfigs with
  group/world-readable bits are rejected
- symlink hardening: pull-secret, SSH key, flux deploy-key, secret
  store, download, and copy destinations all refuse symlinks
- the run lock works under sudo: the lockfile is chowned back to the
  invoking user, keeps a stable inode across runs, and records the
  hostname to diagnose NFS contention
- Ctrl-C soft-cancels subprocesses (SIGINT with a bounded wait) and
  cancellation identity survives into reported errors
- `kubeconfig` writes through stdout so shell redirection works

### Removed
- macOS / darwin support. The deploy phase shells out to `dnf`/`apt`,
  `firewall-cmd`, `nmcli`, and `systemctl`, none of which exist on
  macOS — the doctor and wizard would run on a Mac but the actual
  deploy never could. Goreleaser darwin targets, the Homebrew tap,
  and the install script's macOS branch are all gone.
- Windows build target from Makefile (never supported, now explicitly
  dropped)
- internal audit and agent-workflow artifacts (.claude/, docs/superpowers/)
  are no longer tracked in the repository
- dead option knobs that never gated behavior (setup auto-download and
  verbose switches, install bootstrap-log streaming, terraform
  constructor-level var-file override)

## [0.1.0] - TBD

Initial public release.

[Unreleased]: https://github.com/qxtaiba/okdctl/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/qxtaiba/okdctl/releases/tag/v0.1.0
