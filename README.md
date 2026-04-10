# openshitctl

> A single-binary CLI for deploying production-ready OKD (OpenShift Kubernetes
> Distribution) clusters on Proxmox VE, aimed at homelab operators who want a
> real Kubernetes cluster without the operational overhead of writing their
> own Terraform, Ignition, and bootstrap plumbing.

[![CI](https://github.com/qxtaiba/okd-proxmox-cli/actions/workflows/ci.yml/badge.svg?branch=develop)](https://github.com/qxtaiba/okd-proxmox-cli/actions/workflows/ci.yml)
[![CodeQL](https://github.com/qxtaiba/okd-proxmox-cli/actions/workflows/codeql.yml/badge.svg)](https://github.com/qxtaiba/okd-proxmox-cli/actions/workflows/codeql.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/qxtaiba/okd-proxmox-cli)](https://goreportcard.com/report/github.com/qxtaiba/okd-proxmox-cli)
[![Release](https://img.shields.io/github/v/release/qxtaiba/okd-proxmox-cli?sort=semver)](https://github.com/qxtaiba/okd-proxmox-cli/releases)

`openshitctl` walks you through an interactive TUI wizard, generates the full
set of OKD install artifacts (install-config, manifests, ignition, custom ISOs),
provisions VMs via Terraform on your Proxmox host, sits the cluster up through
bootstrap, and hands you a working cluster with console access. Then it cleans
up after itself when you're done.

It's designed for the homelab operator who has one or a few Proxmox nodes and
wants a Kubernetes platform that behaves like the ones at work, without
becoming a second job.

## Features

- **Interactive wizard TUI** — no YAML-by-hand; the wizard writes a
  validated config and remembers your answers across runs
- **Data-driven phases** — setup, install, post-install, destroy, and
  cleanup each run as ordered step lists with rollback-on-failure semantics
- **Addon system** — built-in catalog for Flux, cert-manager, storage
  classes, external-secrets, and more (opt-in per cluster)
- **Automated preflight** — `openshitctl doctor` verifies your host is
  ready before any destructive operation
- **Diagnostic bundles** — `openshitctl diag` collects a sanitized
  support bundle you can attach to an issue
- **kube-vip for API VIP** — no reliance on external load balancers for the
  API server; HAProxy handles ingress
- **Compact cluster support** — 3-master / 0-worker topologies for small
  homelabs are a first-class configuration
- **Zeroizing credential handling** — Proxmox credentials are wiped from
  memory after use and never logged
- **Supply chain hygiene** — sigstore-signed releases, SBOM attached to
  every release, SLSA provenance, reproducible builds

## Compatibility

<!-- COMPAT:START -->
<!-- This section is generated from docs/compatibility.yaml. Run `make compat` to regenerate. -->

| Component | Tested | Known broken / unsupported |
|-----------|--------|----------------------------|
| Proxmox VE | 8.0, 8.1, 8.2 | `<8.0` (API breaking changes in the pve-api-go client) |
| OKD | 4.15.0, 4.16.0 | `<4.13` (ignition format v3 changes not handled) |
| Host OS | Debian 12, Fedora 39 | — |
| Storage backend | LVM-thin, directory | `Ceph` (not tested; PRs welcome) |
| Firewall | firewalld, ufw | `nftables (direct)` (not tested; firewalld and ufw both use nftables under the hood) |
| Host architecture | amd64, arm64 | `Windows` (not supported by design — this is a Linux/macOS tool) |

<!-- COMPAT:END -->

If your configuration isn't on the list, it may still work — the list reflects
what's actively tested, not what's theoretically supported. PRs adding new
tested combinations are welcome; see `docs/compatibility.yaml`.

## Quickstart

### 1. Install

**Homebrew (macOS / Linux):**

```sh
brew install qxtaiba/tap/openshitctl
```

**Shell installer (any Linux / macOS):**

```sh
curl -sfL https://raw.githubusercontent.com/qxtaiba/okd-proxmox-cli/main/scripts/install.sh | sh
```

The installer auto-detects your OS and architecture, downloads the matching
release from GitHub, verifies its SHA256 against the published checksum file,
and drops the binary in `/usr/local/bin`.

**Direct binary:**

Grab a release archive from
[GitHub Releases](https://github.com/qxtaiba/okd-proxmox-cli/releases) for
your OS/arch, extract, and put the `openshitctl` binary somewhere on your
`$PATH`. Each release ships with a `SHA256SUMS` file and sigstore signatures
(see [Verifying a release](#verifying-a-release)).

**From source:**

```sh
git clone https://github.com/qxtaiba/okd-proxmox-cli
cd okd-proxmox-cli
make build
sudo install -m 0755 bin/openshitctl /usr/local/bin/
```

> Do **not** run `openshitctl` as root — it refuses to start under `sudo`
> and escalates privileges internally only for the specific commands that
> need them (`nmcli`, `firewall-cmd`, `systemctl`, etc.).

### 2. Get an OKD pull secret

OKD uses the same release-image infrastructure as OpenShift, and it needs
a pull secret to pull from `quay.io/openshift-release-dev` during install.
The pull secret is **free** — you just need a Red Hat account.

1. Go to <https://console.redhat.com/openshift/install/pull-secret>
2. Sign in with a (free) Red Hat Developer account
3. Click **Download pull secret**
4. Save the file to `~/.openshitctl/pull-secret.json`

You'll reference this path in the wizard when it asks for the pull secret.
The file is a JSON object with auth tokens for the registries OKD pulls
from; it carries no PII and is safe to store on a homelab host.

### 3. Check your environment

```sh
openshitctl doctor
```

This runs the preflight checks: OS detection, required binaries, free ports,
sudo capability, pull-secret validity, disk space, Proxmox API reachability.
Fix anything it complains about before proceeding — it's much cheaper to
fix environment problems now than halfway through a bootstrap.

### 4. Configure and deploy

```sh
openshitctl deploy
```

On a fresh run, this launches the wizard. Answer the prompts for cluster
name, domain, node count, Proxmox endpoint, networking, storage, pull
secret path, and any addons you want. The wizard writes
`openshitctl.yaml` alongside a `.env` file for Proxmox credentials
(so you never commit secrets to the config file).

Subsequent runs will reuse the existing `openshitctl.yaml` — pass
`--config path/to/another.yaml` to manage multiple clusters from the same
machine.

The full deploy sequence runs through:

1. **Setup** — install host packages, download tools, generate install
   configs, build custom CoreOS ISOs, configure HAProxy/DNS/firewall
2. **Install** — run Terraform to create the VMs on Proxmox, wait for
   bootstrap, approve CSRs, wait for cluster operators
3. **Post-install** — clean up the bootstrap node, update ingress
   controllers to use LoadBalancer IPs, install any enabled addons

You'll see per-step progress in the TUI, and get the console URL, API URL,
and DNS records you need to point your DNS at when the cluster comes up.

## Configuration

The wizard is the recommended entry point, but if you want to edit YAML
directly, see the three reference configs under `configs/examples/`:

- [`configs/examples/minimal.yaml`](configs/examples/minimal.yaml) — the
  smallest compact cluster (3 masters, 0 workers)
- [`configs/examples/production.yaml`](configs/examples/production.yaml) —
  a 3 master / 3 worker layout with external LB considerations
- [`configs/examples/media-server.yaml`](configs/examples/media-server.yaml) —
  a homelab-flavored config with storage-heavy workers

Proxmox credentials live in a `.env` file next to the config, never in
the YAML itself. See `handleCredentials` in `internal/cli/helpers.go` for
the exact env variable names (`PROXMOX_VE_USERNAME`, `PROXMOX_VE_PASSWORD`,
`PROXMOX_VE_API_TOKEN`, `PROXMOX_VE_ENDPOINT`).

## Commands

```
openshitctl deploy            Deploy a new cluster (launches wizard on first run)
openshitctl destroy           Tear down a cluster, cleaning up VMs and host config
openshitctl update-ingress    Switch ingress controllers from HostNetwork to LoadBalancer IPs
openshitctl doctor            Run preflight checks against the local environment
openshitctl diag              Collect a sanitized diagnostic bundle as a tarball
openshitctl version           Print version, git commit, build date, platform
```

Use `--help` on any command for flags and details.

## How it works

At a high level, `openshitctl` is organized around a **phase/step model**:

```
deploy = setup → install → post-install
destroy = destroy → cleanup
```

Each phase is a list of `StepDef` values ordered by dependency; the shared
`distribution.Orchestrator` runs them with progress, logging, and
rollback-on-failure. The phases live under `internal/distribution/okd/`:

- [`setup/`](internal/distribution/okd/setup/) — host packages, ignition
  generation, ISO customization, HAProxy/DNS/firewall configuration
- [`install/`](internal/distribution/okd/install/) — Terraform orchestration,
  bootstrap monitoring, CSR approval, cluster operator wait
- [`postinstall/`](internal/distribution/okd/postinstall/) — bootstrap cleanup,
  ingress migration, addon installation
- [`destroy/`](internal/distribution/okd/destroy/) — Terraform destroy plus
  file/service cleanup
- [`cleanup/`](internal/distribution/okd/cleanup/) — standalone cleanup helpers
  invoked both by destroy and as a subcommand

See [`docs/architecture/`](docs/architecture/) for deeper dives on the phase
model, addon system, and wizard.

## Verifying a release

Every tagged release produces:

- `openshitctl_<version>_<os>_<arch>.{tar.gz,zip}` — the binary archive
- `SHA256SUMS` — checksums of all archives
- `SHA256SUMS.sig` + `SHA256SUMS.pem` — sigstore keyless signature + cert
- `openshitctl.sbom.json` — CycloneDX SBOM of all dependencies
- `openshitctl.intoto.jsonl` — SLSA build provenance attestation

### Verify the SHA256

```sh
sha256sum --check SHA256SUMS 2>&1 | grep openshitctl_<version>_<os>_<arch>
```

### Verify the sigstore signature (no keys needed)

```sh
cosign verify-blob \
  --signature SHA256SUMS.sig \
  --certificate SHA256SUMS.pem \
  --certificate-identity-regexp 'https://github.com/qxtaiba/okd-proxmox-cli/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  SHA256SUMS
```

This proves the checksums file was produced by a GitHub Actions workflow
in this repository at release time — no maintainer private key, no trust in
the release page markup, no trust in any CDN between you and GitHub.

### Reproduce the build from source

The release binaries are built with `-trimpath` and deterministic ldflags,
so a local build from the tagged commit should produce a byte-identical
binary for the same `GOOS`/`GOARCH`:

```sh
git checkout v<version>
CGO_ENABLED=0 go build \
  -trimpath \
  -ldflags "-s -w \
    -X github.com/qxtaiba/okd-proxmox-cli/pkg/version.Version=v<version> \
    -X github.com/qxtaiba/okd-proxmox-cli/pkg/version.GitCommit=<short-commit> \
    -X github.com/qxtaiba/okd-proxmox-cli/pkg/version.BuildDate=<release-date>" \
  -o openshitctl ./cmd/openshitctl
sha256sum openshitctl  # compare with SHA256SUMS
```

## Clean uninstall

If you want to remove `openshitctl` and everything it created on your host:

```sh
# 1. Tear down the cluster (this removes VMs on Proxmox and cleans host config)
openshitctl destroy

# 2. Remove any residual work directory
rm -rf ~/okd-install  # or wherever your config's workDir points

# 3. Remove the config and credentials
rm -rf ~/.openshitctl openshitctl.yaml .env

# 4. Remove the binary
sudo rm /usr/local/bin/openshitctl
# or: brew uninstall openshitctl
```

`openshitctl destroy` handles the non-obvious parts: it removes the
dnsmasq drop-in config, tears down the HAProxy config block, removes the
firewall rules it added during setup, and destroys the Terraform-managed
VMs. Pass `--remove-packages` if you also want the system packages
(`haproxy`, `dnsmasq`, `httpd`) uninstalled — they stay by default because
you may be using them for other things.

## Troubleshooting

Before filing an issue, please run:

```sh
openshitctl doctor             # what's wrong with my environment?
openshitctl diag --sanitize    # bundle everything for a bug report
```

### Common failures

- **Bootstrap VM never comes up:** usually a networking issue. Check that
  the ignition URL is reachable from the node network (HAProxy IP, port
  8080, path `/ignition/<role>.ign`). `openshitctl doctor` tests this.
- **`dnsmasq` failed to start:** port 53 is already bound, often by
  `systemd-resolved`. Either disable `systemd-resolved` DNS stub
  (`DNSStubListener=no` in `/etc/systemd/resolved.conf`) or run
  `openshitctl deploy --skip-dns` and handle DNS with your own resolver.
- **`oc` command not found:** the tool auto-installs `oc` into
  `/usr/local/bin`, which may not be on `$PATH` during the setup phase.
  Log out and back in, or re-source your shell rc.
- **Terraform destroy hangs:** Proxmox API under load can drop long-running
  destroy requests. Re-run `openshitctl destroy` — the Terraform state
  is preserved and it picks up where it left off.
- **CSRs never get approved:** check `openshitctl doctor` for clock skew
  between the bastion and nodes, and verify the `oc` binary points at the
  right kubeconfig (`openshitctl` sets `KUBECONFIG` on its internal
  executor; see `internal/distribution/okd/install/phase.go`).

## Versioning and stability

`openshitctl` is pre-1.0 and follows **SemVer with a homelab-safety twist**:

- **Patch releases** (`v0.x.y` → `v0.x.z`) never change on-disk state
  formats, the config file schema, or command behavior
- **Minor releases** (`v0.x.*` → `v0.y.*`) may change the config file
  schema; the `CHANGELOG.md` documents a migration path for every breaking
  change, and `openshitctl` will refuse to run against a config it doesn't
  understand rather than silently corrupting state
- **Post-1.0**, breaking changes are reserved for major releases

Pinning to an exact version is recommended until 1.0. If you use the
`curl | sh` installer, pass `VERSION=v0.1.0` to pin.

## Contributing

Found a bug? Have an idea? PRs are welcome. Before you start:

1. Run the tests: `make test`
2. Run the linter: `make lint`
3. Check for dead code: `deadcode ./cmd/openshitctl`
4. File an issue first if you're planning a significant change — it's
   much faster to align on approach before writing a lot of code

See the issue templates (`.github/ISSUE_TEMPLATE/`) for what to include in
bug reports and feature requests. The PR template
(`.github/PULL_REQUEST_TEMPLATE.md`) has a short checklist for review
readiness.

## License

Copyright 2026 Q Al Nuaimi.

Licensed under the Apache License, Version 2.0 (the "License"); you may not
use this file except in compliance with the License. You may obtain a copy
of the License at <http://www.apache.org/licenses/LICENSE-2.0>. See the
[LICENSE](LICENSE) and [NOTICE](NOTICE) files for details.
