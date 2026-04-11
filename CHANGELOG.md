# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `okdctl doctor` — preflight checks for OS, required binaries,
  free ports, sudo access, pull-secret validity, disk space, Proxmox
  reachability
- Apache-2.0 license
- Architecture documentation under `docs/architecture/`
- Sigstore keyless signing on release artifacts
- SBOM (CycloneDX) attached to each release
- SLSA build provenance attestation
- `.deb` and `.rpm` packages published on release
- `curl | sh` installer script with SHA256 verification
- CodeQL SAST scan on every PR
- Community templates: issue forms, PR template, CODEOWNERS, FUNDING

### Changed
- Release builds now use `-trimpath` for reproducibility
- Project renamed from `openshitctl` (binary) and `okd-proxmox-cli`
  (module path) to `okdctl` for both. Module path is now
  `github.com/qxtaiba/okdctl`. Anyone with the old paths needs to
  update their go.mod and re-clone the renamed GitHub repo.

### Removed
- macOS / darwin support. The deploy phase shells out to `dnf`/`apt`,
  `firewall-cmd`, `nmcli`, and `systemctl`, none of which exist on
  macOS — the doctor and wizard would run on a Mac but the actual
  deploy never could. Goreleaser darwin targets, the Homebrew tap,
  and the install script's macOS branch are all gone.
- Windows build target from Makefile (never supported, now explicitly
  dropped)

## [0.1.0] - TBD

Initial public release.

[Unreleased]: https://github.com/qxtaiba/okdctl/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/qxtaiba/okdctl/releases/tag/v0.1.0
