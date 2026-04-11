# openshitctl

[![CI](https://github.com/qxtaiba/okd-proxmox-cli/actions/workflows/ci.yml/badge.svg?branch=develop)](https://github.com/qxtaiba/okd-proxmox-cli/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

A single-binary CLI that stands up OKD (OpenShift Kubernetes Distribution)
clusters on Proxmox VE. Runs a wizard, generates install configs and
custom CoreOS ISOs, drives Terraform to provision the VMs on your
Proxmox node, waits through bootstrap and cluster-operator stages,
installs any enabled addons, and cleans up after itself on destroy.

Built for the homelab operator with one or two Proxmox nodes who wants
a real Kubernetes cluster that behaves like the ones at work, without
hand-rolling Terraform, Ignition, and bootstrap glue.

## Compatibility

| Component | Tested | Known broken / unsupported |
|-----------|--------|----------------------------|
| Proxmox VE | 8.0, 8.1, 8.2 | `<8.0` (API breaking changes in the pve-api-go client) |
| OKD | 4.15.0, 4.16.0 | `<4.13` (ignition format v3 changes not handled) |
| Host OS | Debian 12, Fedora 39 | — |
| Storage backend | LVM-thin, directory | `Ceph` (not tested; PRs welcome) |
| Firewall | firewalld, ufw | `nftables (direct)` (not tested) |
| Host architecture | amd64, arm64 | Windows (not supported by design — this is a Linux/macOS tool) |

Not on the list doesn't mean it won't work — this reflects what's
actively tested, nothing more. PRs adding tested combinations to this
table are welcome.

## Install

**Homebrew:**

```sh
brew install qxtaiba/tap/openshitctl
```

**curl | sh** (auto-detects OS/arch, verifies SHA256):

```sh
curl -sSfL https://raw.githubusercontent.com/qxtaiba/okd-proxmox-cli/main/scripts/install.sh | sh
```

**`.deb` / `.rpm`** from the
[releases page](https://github.com/qxtaiba/okd-proxmox-cli/releases) for
apt/dnf users.

**From source:**

```sh
git clone https://github.com/qxtaiba/okd-proxmox-cli
cd okd-proxmox-cli
make build
sudo install -m 0755 bin/openshitctl /usr/local/bin/
```

Releases are signed with sigstore keyless signing and ship with a
CycloneDX SBOM and SLSA build provenance. See
[Verifying a release](#verifying-a-release) to check what you
downloaded.

Don't run `openshitctl` as root. It refuses to start under `sudo` and
escalates internally only for the specific commands that need it
(`nmcli`, `firewall-cmd`, `systemctl`).

## Usage

```
openshitctl deploy           run the wizard, then deploy the cluster
openshitctl destroy          tear down a cluster
openshitctl update-ingress   switch ingress controllers to LoadBalancer IPs
openshitctl doctor           environment preflight check
openshitctl version          print version, git commit, build date
```

On first run, `deploy` launches the interactive wizard and writes
`openshitctl.yaml` alongside a `.env` file for Proxmox credentials.
Subsequent runs reuse the existing config. Pass `--config other.yaml`
to manage multiple clusters from the same machine.

A deploy walks through three phases:

1. **setup** — install host packages, download external tools (`oc`,
   `openshift-install`, `terraform`), generate install configs and k8s
   manifests, build custom CoreOS ISOs, and configure HAProxy, DNS, and
   firewall on the bastion
2. **install** — run Terraform to provision VMs on Proxmox, wait for
   bootstrap, approve CSRs, wait for cluster operators to come up
3. **post-install** — clean up the bootstrap node, migrate ingress to
   LoadBalancer IPs if you have an LB provider, install any enabled
   addons

Each phase is a sequence of steps with rollback on failure. Re-running
`deploy` after an interruption picks up where it left off. Skip flags
(`--skip-terraform`, `--skip-isos`, `--skip-haproxy`, `--skip-dns`) let
you bring your own parts of the stack.

The phase model, addon system, and wizard internals are documented in
[`docs/architecture/`](docs/architecture/).

## Configuration

The wizard is the easiest way to generate a config. If you prefer
editing YAML directly, reference configs for common setups live in
[`configs/examples/`](configs/examples/):

- `minimal.yaml` — 3 control-plane nodes, 0 workers (compact cluster)
- `production.yaml` — 3 control-plane, 3 worker layout
- `media-server.yaml` — homelab-flavored with storage-heavy workers

Proxmox credentials live in a `.env` file next to the config, never in
the YAML itself. The env var names:

```
PROXMOX_VE_ENDPOINT
PROXMOX_VE_USERNAME
PROXMOX_VE_PASSWORD    # or PROXMOX_VE_API_TOKEN
```

### Getting an OKD pull secret

OKD pulls release images from the same registries as OpenShift and
needs a pull secret. It's free — you just need a Red Hat account.

1. Log in at
   [console.redhat.com/openshift/install/pull-secret](https://console.redhat.com/openshift/install/pull-secret)
2. Click **Download pull secret**
3. Save it somewhere on your host. The wizard defaults to
   `~/pull-secret.json`; you can also pick another location and
   reference it during the wizard's file-paths step.
4. The file is a JSON object with registry auth tokens — no PII, safe
   to store locally.

## Troubleshooting

Before filing an issue, run `openshitctl doctor` and attach the output.
It catches most of the common failures ahead of time.

- **Bootstrap VM never comes up.** Usually a networking issue. Verify
  the ignition URL is reachable from the node network (HAProxy IP,
  port 8080, path `/ignition/<role>.ign`). Doctor probes this.
- **`dnsmasq` fails to start on port 53.** `systemd-resolved` grabs
  port 53 on most modern distros. Either disable its DNS stub
  (`DNSStubListener=no` in `/etc/systemd/resolved.conf`) or run
  `openshitctl deploy --skip-dns` and handle DNS externally.
- **`oc` not found mid-setup.** The tool auto-installs `oc` into
  `/usr/local/bin`, which may not be on your `$PATH` during the same
  shell session. Re-source your shell rc and retry.
- **Terraform destroy hangs.** The Proxmox API under load can drop
  long-running destroy requests. Re-run `openshitctl destroy` — the
  state is preserved and it picks up where it left off.
- **Clock skew during CSR approval.** The CSR flow refuses to approve
  certs from nodes whose clock differs from the bastion's. Run
  `ntpdate` on both and retry.

## Uninstall

```sh
openshitctl destroy                          # tear down the cluster
rm -rf ~/okd-install openshitctl.yaml .env   # residual state
sudo rm /usr/local/bin/openshitctl           # or: brew uninstall openshitctl
```

`destroy` removes the dnsmasq drop-in, the HAProxy config block, the
firewall rules openshitctl added during setup, and the
Terraform-provisioned VMs. Pass `--remove-packages` if you also want
the system packages uninstalled (`haproxy`, `dnsmasq`, `httpd`) — by
default they stay since you may be using them for other things.

## Verifying a release

Every tagged release ships:

- `openshitctl_<version>_<os>_<arch>.{tar.gz,zip}` — binary archive
- `SHA256SUMS`, `SHA256SUMS.sig`, `SHA256SUMS.pem` — sigstore keyless
  signature
- `openshitctl.sbom.json` — CycloneDX SBOM
- `openshitctl.intoto.jsonl` — SLSA provenance attestation

Check the signature without managing any keys:

```sh
cosign verify-blob \
  --signature SHA256SUMS.sig \
  --certificate SHA256SUMS.pem \
  --certificate-identity-regexp 'https://github.com/qxtaiba/okd-proxmox-cli/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  SHA256SUMS
```

This proves the checksums file was produced by a GitHub Actions
workflow in this repository at release time — no maintainer private
keys, no trust in the release page markup, no trust in any CDN between
you and GitHub.

Binaries are built with `-trimpath` and deterministic ldflags, so
`make build` from the tagged commit should produce a byte-identical
binary. Compare `sha256sum bin/openshitctl` against the published
`SHA256SUMS`.

## Status

Pre-1.0. Expect the config schema to change between minor versions.
The `CHANGELOG` has migration notes for breaking changes, and the tool
refuses to run against a config it doesn't understand rather than
silently corrupting state. Pin to a specific version until 1.0.

## Contributing

Bug reports and PRs welcome. Run `make test && make lint` before
submitting. The issue forms in `.github/ISSUE_TEMPLATE/` ask for the
info I need to reproduce; filling them out usually saves a round trip.

## License

Apache-2.0. Copyright 2026 Q Al Nuaimi. See [LICENSE](LICENSE) and
[NOTICE](NOTICE).
