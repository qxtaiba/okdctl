# openshitctl

[![CI](https://github.com/qxtaiba/okd-proxmox-cli/actions/workflows/ci.yml/badge.svg?branch=develop)](https://github.com/qxtaiba/okd-proxmox-cli/actions/workflows/ci.yml)
[![CodeQL](https://github.com/qxtaiba/okd-proxmox-cli/actions/workflows/codeql.yml/badge.svg)](https://github.com/qxtaiba/okd-proxmox-cli/actions/workflows/codeql.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/qxtaiba/okd-proxmox-cli)](https://goreportcard.com/report/github.com/qxtaiba/okd-proxmox-cli)
[![Release](https://img.shields.io/github/v/release/qxtaiba/okd-proxmox-cli?sort=semver)](https://github.com/qxtaiba/okd-proxmox-cli/releases)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

openshitctl provisions OKD clusters on Proxmox VE from an interactive wizard.
It's for the homelab operator with one or two Proxmox nodes who wants a real
Kubernetes cluster without hand-rolling Terraform, Ignition, and bootstrap glue.

## Compatibility

| Component | Tested | Known broken / unsupported |
|-----------|--------|----------------------------|
| Proxmox VE | 8.0, 8.1, 8.2 | `<8.0` (API breaking changes in the pve-api-go client) |
| OKD | 4.15.0, 4.16.0 | `<4.13` (ignition format v3 changes not handled) |
| Host OS | Debian 12, Fedora 39 | — |
| Storage backend | LVM-thin, directory | `Ceph` (not tested; PRs welcome) |
| Firewall | firewalld, ufw | `nftables (direct)` (not tested) |
| Host architecture | amd64, arm64 | Windows (not supported by design — this is a Linux/macOS tool) |

The table reflects what's actively tested. Not on the list doesn't mean it
won't work — PRs adding tested combinations welcome.

## Install

**Homebrew:**

```sh
brew install qxtaiba/tap/openshitctl
```

**curl | sh** (auto-detects OS/arch, verifies SHA256):

```sh
curl -sSfL https://raw.githubusercontent.com/qxtaiba/okd-proxmox-cli/main/scripts/install.sh | sh
```

**`.deb` / `.rpm`** from the [releases page](https://github.com/qxtaiba/okd-proxmox-cli/releases)
for apt/dnf users.

**From source:**

```sh
git clone https://github.com/qxtaiba/okd-proxmox-cli
cd okd-proxmox-cli
make build
sudo install -m 0755 bin/openshitctl /usr/local/bin/
```

Releases are sigstore-signed (keyless) and ship with a CycloneDX SBOM and SLSA
build provenance. Verify with `cosign verify-blob` — see
[Verifying a release](#verifying-a-release).

Don't run `openshitctl` as root. It refuses to start under `sudo` and
escalates internally for the commands that need it (`nmcli`, `firewall-cmd`,
`systemctl`).

## Usage

```
openshitctl deploy           run the wizard, then deploy the cluster
openshitctl destroy          tear down a cluster
openshitctl update-ingress   switch ingress controllers to LoadBalancer IPs
openshitctl doctor           environment preflight check
openshitctl version          print version, git commit, build date
```

First run of `deploy` launches the wizard and writes `openshitctl.yaml` plus
a `.env` for Proxmox credentials. Later runs reuse the existing config.
`--config other.yaml` manages multiple clusters from one machine.

A deploy runs three phases:

1. **setup** — host packages, download `oc`/`openshift-install`/`terraform`,
   generate install configs and k8s manifests, build custom CoreOS ISOs,
   configure HAProxy/DNS/firewall on the bastion
2. **install** — Terraform brings up the VMs on Proxmox, wait for bootstrap,
   approve CSRs, wait for cluster operators
3. **post-install** — remove the bootstrap node, migrate ingress to
   LoadBalancer IPs if an LB provider is installed, install any enabled addons

Each phase is a sequence of steps with rollback on failure. Re-running
`deploy` after an interruption picks up where it left off. `--skip-terraform`,
`--skip-isos`, `--skip-haproxy`, and `--skip-dns` let you bring your own parts
of the stack.

Phase internals, addon system, and wizard architecture live in
[`docs/architecture/`](docs/architecture/).

## Configuration

Use the wizard. If you'd rather edit YAML directly, reference configs live in
[`configs/examples/`](configs/examples/):

- `minimal.yaml` — 3 control-plane nodes, 0 workers (compact cluster)
- `production.yaml` — 3 control-plane, 3 worker layout
- `media-server.yaml` — homelab setup with storage-heavy workers

Proxmox credentials live in a `.env` file next to the config, never in the
YAML. Env vars: `PROXMOX_VE_ENDPOINT`, `PROXMOX_VE_USERNAME`,
`PROXMOX_VE_PASSWORD` (or `PROXMOX_VE_API_TOKEN`).

### OKD pull secret

OKD pulls release images from the same registries as OpenShift. You need a
(free) Red Hat account.

1. Log in at [console.redhat.com/openshift/install/pull-secret](https://console.redhat.com/openshift/install/pull-secret)
2. Click **Download pull secret**
3. Save it anywhere. The wizard defaults to `~/pull-secret.json`.

## Troubleshooting

Run `openshitctl doctor` first. It catches most common failures and its
output goes in bug reports.

- **Bootstrap VM never comes up.** Networking. The ignition URL must be
  reachable from the node network (HAProxy IP, port 8080, path
  `/ignition/<role>.ign`). Doctor probes this.
- **`dnsmasq` fails on port 53.** `systemd-resolved` has it. Either set
  `DNSStubListener=no` in `/etc/systemd/resolved.conf`, or run
  `openshitctl deploy --skip-dns` and handle DNS yourself.
- **`oc` not found mid-setup.** openshitctl installs it into
  `/usr/local/bin`, which isn't on `$PATH` in the current shell until you
  re-source your rc.
- **Terraform destroy hangs.** Proxmox API drops long-running destroy
  requests under load. Re-run `openshitctl destroy` — state is preserved.
- **CSR approval fails on clock skew.** Nodes whose clock differs from the
  bastion's get their certs refused. Run `ntpdate` on both and retry.

## Uninstall

```sh
openshitctl destroy                          # tear down the cluster
rm -rf ~/okd-install openshitctl.yaml .env   # residual state
sudo rm /usr/local/bin/openshitctl           # or: brew uninstall openshitctl
```

`destroy` removes the dnsmasq drop-in, HAProxy config block, firewall rules
openshitctl added, and the Terraform-provisioned VMs. `--remove-packages`
also uninstalls `haproxy`, `dnsmasq`, `httpd` — by default they stay, since
you may be using them for other things.

## Verifying a release

Every tagged release ships:

- `openshitctl_<version>_<os>_<arch>.{tar.gz,zip}` — binary archive
- `SHA256SUMS`, `SHA256SUMS.sig`, `SHA256SUMS.pem` — sigstore keyless signature
- `openshitctl.sbom.json` — CycloneDX SBOM
- `openshitctl.intoto.jsonl` — SLSA build provenance

Verify the signature without managing any keys:

```sh
cosign verify-blob \
  --signature SHA256SUMS.sig \
  --certificate SHA256SUMS.pem \
  --certificate-identity-regexp 'https://github.com/qxtaiba/okd-proxmox-cli/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  SHA256SUMS
```

This proves the checksums file came from a GitHub Actions workflow in this
repository — no maintainer private keys, no trust in the release page markup,
no trust in any CDN between you and GitHub.

Binaries are built with `-trimpath` and deterministic ldflags, so `make
build` from the tagged commit produces a byte-identical binary.
`sha256sum bin/openshitctl` should match `SHA256SUMS`.

## Status

Pre-1.0. Config schema changes between minor versions. The `CHANGELOG` has
migration notes for every breaking change; the tool refuses to run against a
config it doesn't understand rather than silently corrupt state. Pin to a
specific version until 1.0.

## Contributing

PRs welcome. Run `make test && make lint` before submitting. The issue forms
in `.github/ISSUE_TEMPLATE/` ask for the info I need to reproduce — filling
them out saves a round trip.

## License

Apache-2.0. Copyright 2026 Q Al Nuaimi. See [LICENSE](LICENSE) and
[NOTICE](NOTICE).
