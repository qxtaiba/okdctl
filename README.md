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

**curl | bash** (verifies SHA256):

```bash
curl -sSfL https://raw.githubusercontent.com/qxtaiba/okdctl/main/scripts/install.sh | bash
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
```

Don't run `okdctl` as root. It refuses to start under `sudo` and
escalates internally for the commands that need it (`nmcli`, `firewall-cmd`,
`systemctl`).

## Usage

```
okdctl addon            manage cluster addons
okdctl cleanup          remove OKD cluster artifacts without destroying infrastructure
okdctl completion       generate shell completion script
okdctl config           inspect okdctl configuration
okdctl debug-bundle     collect a support bundle for troubleshooting
okdctl deploy           deploy a Kubernetes cluster
okdctl describe         drill into a specific node or addon
okdctl destroy          destroy a Kubernetes cluster
okdctl doctor           check that your environment is ready to deploy a cluster
okdctl kubeconfig       print or export the cluster kubeconfig
okdctl releases         query available OKD versions
okdctl status           print a post-deploy cluster summary
okdctl update-ingress   switch ingress DNS from HAProxy to LoadBalancer IPs
okdctl version          print version, git commit, build date
```

Full command reference: [`docs/cli/okdctl.md`](docs/cli/okdctl.md).
Exit codes and shell-script idioms: [`docs/cli/exit-codes.md`](docs/cli/exit-codes.md).

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
`deploy` after an interruption picks up where it left off. If the work
directory holds a completed cluster, run `okdctl destroy` first; `--fresh`
force-wipes it without destroying (credentials will be lost).

Phase internals, addon system, and wizard architecture live in
[`docs/architecture/`](docs/architecture/). Per-addon reference:
[Flux](docs/addons/flux.md),
[1Password Secret Store](docs/addons/secretstore.md).

## Configuration

Use the wizard. If you'd rather edit YAML directly, reference configs live in
[`configs/examples/`](configs/examples/):

- `minimal.yaml` — 1 control-plane node, 0 workers (single-node cluster)
- `production.yaml` — 3 control-plane, 5 worker layout
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
- **`dnsmasq` fails on port 53.** `systemd-resolved` has it. Set
  `DNSStubListener=no` in `/etc/systemd/resolved.conf` and restart
  `systemd-resolved`, then retry `okdctl deploy`.
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
okdctl added, the Terraform-provisioned VMs, and the `haproxy`, `dnsmasq`,
and `httpd` packages it installed.

## Verifying a release

Every tagged release ships:

- `okdctl_<version>_linux_<arch>.tar.gz` — binary archive (amd64 + arm64)
- `SHA256SUMS`, `SHA256SUMS.sig`, `SHA256SUMS.pem` — sigstore keyless signature
- `okdctl_<version>_linux_<arch>.sbom.json` — CycloneDX SBOM (binary archive)
- `okdctl_<version>_linux_<arch>.pkg.sbom.json` — CycloneDX SBOM (apt/rpm package)
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

Each release also carries a GitHub artifact attestation (SLSA build
provenance recorded in the repository's attestations log). Verify any
shipped file with:

```sh
gh attestation verify <file> --repo qxtaiba/okdctl
```

This checks the attestation against GitHub's Sigstore instance and confirms
the artifact was produced by a workflow in this repository. Requires the
[GitHub CLI](https://cli.github.com/) (`gh`).

Binaries are built with `-trimpath` and deterministic ldflags, so `make
build` from the tagged commit produces a byte-identical binary.
`sha256sum bin/okdctl` should match `SHA256SUMS`.

## Status

Pre-1.0. The schema changes between minor versions, and the tool refuses to
run against a schema it doesn't understand rather than corrupt state silently.
The [`CHANGELOG`](CHANGELOG.md) documents every break. Pin a version until 1.0.

## Security considerations

### Ignition pull-secret exposure window

During bootstrap (approximately 15–30 minutes), Apache on the bastion serves
`bootstrap.ign`, `master.ign`, and `worker.ign` over HTTP on port 8080. These
files embed the OKD pull-secret JSON in plain text.

okdctl binds Apache to `http_server.ignition_server_ip` (the bridge IP that FCOS
nodes reference in their kargs ignition URL) rather than `0.0.0.0`, which removes
the risk on interfaces that machine-network nodes cannot reach. The residual window:
any host that can reach the bastion bridge IP on port 8080 during bootstrap can
retrieve the ignition files and harvest the pull-secret.

Mitigations:
- Ensure the bastion bridge network is isolated from untrusted hosts (VLAN, private
  bridge, or Proxmox SDN zone) before running `okdctl deploy`.
- After `okdctl deploy` completes, run `okdctl cleanup` which removes the
  ignition files from the web root.
- A future enhancement (tracked in the roadmap) will add a
  firewalld/iptables INPUT rule scoping port 8080 to `networking.machine_cidr`.

### SSH/SCP host-key trust on first run (TOFU window)

The first `okdctl deploy` run scps CoreOS ISOs to the Proxmox host using
`-o StrictHostKeyChecking=accept-new`, which trusts and pins the Proxmox host
key without prior verification. Every subsequent SSH/SCP call (Proxmox shell
commands, ISO removal) reuses that cached key.

A machine-in-the-middle on the bastion-to-Proxmox path during the very first
SCP call can substitute an attacker key that is then trusted for the lifetime
of the cluster.

For deterministic verification, set `provider.proxmox.ssh_host_fingerprint`
in `okdctl.yaml` (`SHA256:<base64>` format from `ssh-keygen -lf
/etc/ssh/ssh_host_ed25519_key.pub` or the Proxmox UI) so every subsequent
SSH/SCP call refuses on mismatch. Set
`provider.proxmox.require_pinned_fingerprint: true` to fail closed when the
pin is absent.

Additional mitigations:

- Run `okdctl deploy` from a bastion with a trusted L2 path to the Proxmox
  host (no NAT or L3 hop an attacker can position on).
- Before the first deploy, manually SSH to the Proxmox host and verify its
  fingerprint out-of-band (Proxmox UI → Node → Shell → `ssh-keygen -lf
  /etc/ssh/ssh_host_ed25519_key.pub`). Once the correct entry is in
  `~/.ssh/known_hosts`, `accept-new` will not override it.

## Contributing

PRs welcome. Run `make test && make lint` before submitting. The issue forms
in `.github/ISSUE_TEMPLATE/` ask for the info I need to reproduce — filling
them out saves a round trip.

## Release checklist

Before tagging a release, regenerate the CLI reference and commit any
drift:

```sh
make docs
git add docs/cli/
git commit -m "docs(cli): regenerate reference for <version>"
```

## License

Apache-2.0. Copyright 2026 Q Al Nuaimi. See [LICENSE](LICENSE) and
[NOTICE](NOTICE).
