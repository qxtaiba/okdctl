# okdctl

[![CI](https://github.com/qxtaiba/okdctl/actions/workflows/ci.yml/badge.svg?branch=develop)](https://github.com/qxtaiba/okdctl/actions/workflows/ci.yml)
[![CodeQL](https://github.com/qxtaiba/okdctl/actions/workflows/codeql.yml/badge.svg)](https://github.com/qxtaiba/okdctl/actions/workflows/codeql.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/qxtaiba/okdctl)](https://goreportcard.com/report/github.com/qxtaiba/okdctl)
[![Release](https://img.shields.io/github/v/release/qxtaiba/okdctl?sort=semver)](https://github.com/qxtaiba/okdctl/releases)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

okdctl provisions OKD clusters on Proxmox VE from an interactive wizard.
It's for the homelab operator with one or two Proxmox nodes who wants a real
Kubernetes cluster without hand-rolling Terraform, Ignition, and bootstrap glue.

## What it isn't

- Not a managed service, not a wrapper around the Red Hat Assisted Installer.
- Not k3s or microk8s — this is upstream OKD with the full operator surface.
- Not multi-cluster or multi-tenant; one cluster per `okdctl.yaml`.
- Not a production OKD distribution. Pre-1.0; pin your version.

okdctl phones home to nothing. No telemetry, no analytics, no update pings.

## Install

okdctl is **Linux-only**. The deploy phase shells out to `dnf`/`apt`,
`firewall-cmd`, `nmcli`, and `systemctl` — none of which exist on macOS or
Windows. Install on the bastion host you intend to deploy from.

**curl | sh** (verifies SHA256):

```sh
curl -sSfL https://raw.githubusercontent.com/qxtaiba/okdctl/main/scripts/install.sh | sh
```

**`.deb` / `.rpm`** from the [releases page](https://github.com/qxtaiba/okdctl/releases)
for apt/dnf users.

**From source:**

```sh
git clone https://github.com/qxtaiba/okdctl
cd okdctl
make build
sudo install -m 0755 bin/okdctl /usr/local/bin/
```

Releases are sigstore-signed (keyless) and ship with a CycloneDX SBOM and SLSA
build provenance. Verify with `cosign verify-blob` — see
[Verifying a release](#verifying-a-release).

### Shell completion

Generate and install a completion script for your shell:

```sh
# bash — persistent
okdctl completion bash > /etc/bash_completion.d/okdctl

# bash — current session only
source <(okdctl completion bash)

# zsh — persistent (pick a dir in $fpath)
okdctl completion zsh > "${fpath[1]}/_okdctl"

# zsh — current session only
source <(okdctl completion zsh)

# fish
okdctl completion fish > ~/.config/fish/completions/okdctl.fish

# powershell — add to $PROFILE for persistence
okdctl completion powershell | Out-String | Invoke-Expression
```

Don't run `okdctl` as root. It refuses to start under `sudo` and
escalates internally for the commands that need it (`nmcli`, `firewall-cmd`,
`systemctl`).

## Usage

```
okdctl deploy           run the wizard, then deploy the cluster
okdctl destroy          tear down a cluster
okdctl update-ingress   switch ingress controllers to LoadBalancer IPs
okdctl doctor           environment preflight check
okdctl version          print version, git commit, build date
```

First run of `deploy` launches the wizard and writes `okdctl.yaml` plus
a `.env` for Proxmox credentials. Later runs reuse the existing config.
`--config other.yaml` manages multiple clusters from one machine.

A deploy runs three phases:

1. **setup** — installs host packages and the tool trio (`oc`,
   `openshift-install`, `terraform`); renders install configs, manifests, and
   custom CoreOS ISOs; configures HAProxy, DNS, and the firewall on the bastion.
2. **install** — Terraform brings the VMs up on Proxmox; okdctl waits for
   bootstrap, approves CSRs, and waits for cluster operators to settle.
3. **post-install** — removes the bootstrap node; migrates ingress to
   LoadBalancer IPs if an LB provider is installed; installs any enabled addons.

Each phase is a sequence of steps with rollback on failure. Re-running
`deploy` after an interruption picks up where it left off. Bring your own infra
with `--skip-terraform` (existing VMs), `--skip-isos` (your own ignition),
`--skip-haproxy`, or `--skip-dns`.

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

You don't need a Red Hat account. The OKD project's own
[README](https://github.com/openshift/okd) prescribes a dummy pull secret that
`openshift-install` accepts — the installer only schema-validates at prompt
time, so this works:

```json
{"auths":{"fake":{"auth":"aWQ6cGFzcwo="}}}
```

Save it as `~/pull-secret.json` (the wizard's default) and the install runs
without touching `console.redhat.com`. Core platform, ingress, OLM, community
operators, monitoring, and the console all work on the dummy. The
`redhat-operators` catalog stays disabled — which is OKD's default anyway.

**Bring your own** if you run a private registry — same JSON format, real
auth strings for your registry. Using a real `console.redhat.com` pull secret
with OKD may violate Red Hat's subscription terms; the OKD community
recommends against it. See [okd-project/okd discussion #1930](https://github.com/okd-project/okd/discussions/1930)
for the canonical Q&A.

## Troubleshooting

Run `okdctl doctor` first. It catches most common failures and its
output goes in bug reports.

- **Bootstrap VM never comes up.** Networking. The ignition URL must be
  reachable from the node network (HAProxy IP, port 8080, path
  `/ignition/<role>.ign`). Doctor probes this.
- **`dnsmasq` fails on port 53.** `systemd-resolved` has it. Either set
  `DNSStubListener=no` in `/etc/systemd/resolved.conf`, or run
  `okdctl deploy --skip-dns` and handle DNS yourself.
- **`oc` not found mid-setup.** okdctl installs it into
  `/usr/local/bin`, which isn't on `$PATH` in the current shell until you
  re-source your rc.
- **Terraform destroy hangs.** Proxmox API drops long-running destroy
  requests under load. Re-run `okdctl destroy` — state is preserved.
- **CSR approval fails on clock skew.** Nodes whose clock differs from the
  bastion's get their certs refused. Run `ntpdate` on both and retry.

## Uninstall

```sh
okdctl destroy                          # tear down the cluster
rm -rf ~/okd-install okdctl.yaml .env   # residual state
sudo rm /usr/local/bin/okdctl           # or: apt remove okdctl / dnf remove okdctl
```

`destroy` removes the dnsmasq drop-in, HAProxy config block, firewall rules
okdctl added, and the Terraform-provisioned VMs. `--remove-packages`
also uninstalls `haproxy`, `dnsmasq`, `httpd` — by default they stay, since
you may be using them for other things.

## Verifying a release

Every tagged release ships:

- `okdctl_<version>_linux_<arch>.tar.gz` — binary archive (amd64 + arm64)
- `SHA256SUMS`, `SHA256SUMS.sig`, `SHA256SUMS.pem` — sigstore keyless signature
- `okdctl.sbom.json` — CycloneDX SBOM
- `okdctl.intoto.jsonl` — SLSA build provenance

Verify the signature without managing any keys:

```sh
cosign verify-blob \
  --signature SHA256SUMS.sig \
  --certificate SHA256SUMS.pem \
  --certificate-identity-regexp 'https://github.com/qxtaiba/okdctl/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  SHA256SUMS
```

This proves the checksums file came from a GitHub Actions workflow in this
repository — no maintainer private keys, no trust in the release page markup,
no trust in any CDN between you and GitHub.

Binaries are built with `-trimpath` and deterministic ldflags, so `make
build` from the tagged commit produces a byte-identical binary.
`sha256sum bin/okdctl` should match `SHA256SUMS`.

## Status

Pre-1.0. The schema changes between minor versions, and the tool refuses to
run against a schema it doesn't understand rather than corrupt state silently.
The [`CHANGELOG`](CHANGELOG.md) documents every break. Pin a version until 1.0.

## Contributing

PRs welcome. Run `make test && make lint` before submitting. The issue forms
in `.github/ISSUE_TEMPLATE/` ask for the info I need to reproduce — filling
them out saves a round trip.

## License

Apache-2.0. Copyright 2026 Q Al Nuaimi. See [LICENSE](LICENSE) and
[NOTICE](NOTICE).
