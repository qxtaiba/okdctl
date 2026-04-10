# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `openshitctl doctor` — preflight checks for OS, required binaries,
  free ports, sudo access, pull-secret validity, disk space, Proxmox
  reachability
- Apache-2.0 license
- Architecture documentation under `docs/architecture/`
- Compatibility matrix in README
- Sigstore keyless signing on release artifacts
- SBOM (CycloneDX) attached to each release
- SLSA build provenance attestation
- Homebrew tap formula published on release
- `.deb` and `.rpm` packages published on release
- `curl | sh` installer script with SHA256 verification
- CodeQL SAST scan on every PR
- Community templates: issue forms, PR template, CODEOWNERS, FUNDING

### Changed
- Release builds now use `-trimpath` for reproducibility

### Removed
- Windows build target from Makefile (never supported, now explicitly dropped)

## [0.1.0] - TBD

Initial public release.

[Unreleased]: https://github.com/qxtaiba/okd-proxmox-cli/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/qxtaiba/okd-proxmox-cli/releases/tag/v0.1.0
