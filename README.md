# okdctl

[![CI](https://github.com/qxtaiba/okdctl/actions/workflows/ci.yml/badge.svg?branch=develop)](https://github.com/qxtaiba/okdctl/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/qxtaiba/okdctl?sort=semver)](https://github.com/qxtaiba/okdctl/releases)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

okdctl provisions OKD clusters on Proxmox VE from an interactive wizard.
It's for the homelab operator with one or two Proxmox nodes who wants a real
Kubernetes cluster without hand-rolling Terraform, Ignition, and bootstrap
glue.

![okdctl deploy wizard](docs/assets/demo.gif)

## What it isn't

- Not a managed service, and not a wrapper around the Red Hat Assisted
  Installer.
- Not k3s or microk8s. This is upstream OKD with the full operator surface.
- Not multi-cluster or multi-tenant; one cluster per `okdctl.yaml`.
- Not a production OKD distribution. Pre-1.0, the config schema breaks
  between minors, so pin your version. Every break is listed in the
  [release notes](https://github.com/qxtaiba/okdctl/releases).

okdctl collects no telemetry and no analytics. The one request it makes on
its own behalf is an update check: a plain HTTPS GET to `api.github.com`
(`releases/latest`), at most once per 24 hours, cached in
`~/.cache/okdctl/update-check.json`. Builds without a release version skip
it. Set `OKDCTL_NO_UPDATE_CHECK=1` to turn it off.

## Requirements

- A Linux host to deploy from (the bastion), rhel- or debian-family. The
  deploy phase shells out to `dnf`/`apt`, `firewall-cmd`, `nmcli`, and
  `systemctl`, so macOS and Windows can build okdctl but not deploy with it.
- A Proxmox VE node reachable from the bastion over SSH and the API.
- `curl`, `ssh`, and `git` on the bastion, sudo access, and 20 GB of free
  disk in your home directory.

`okdctl doctor` checks all of this before you commit to a deploy.

## Install

**curl | bash** (cosign signature + SHA256):

```bash
curl -sSfL https://raw.githubusercontent.com/qxtaiba/okdctl/develop/scripts/install.sh | bash
```

The installer verifies the sigstore cosign signature on `SHA256SUMS`, then
byte-compares the downloaded archive against that checksum before installing.
That verification requires [cosign](https://docs.sigstore.dev/cosign/installation/)
on your `PATH` — it ships in no base distro, so install it first. To accept
sha256-only trust when cosign is unavailable, set `INSECURE=1`; the SHA256 check
still runs and the skipped signature step is logged loudly.

**`.deb` / `.rpm`** from the [releases page](https://github.com/qxtaiba/okdctl/releases)
for apt/dnf users.

**From source:**

```sh
git clone https://github.com/qxtaiba/okdctl
cd okdctl
make build
sudo install -m 0755 bin/okdctl /usr/local/bin/
```

The releases are sigstore-signed (keyless) and ship with a CycloneDX
SBOM and SLSA build provenance. See
[Verifying a release](#verifying-a-release).

## Quick start

```sh
mkdir my-cluster && cd my-cluster
okdctl deploy
```

The wizard asks for Proxmox credentials and cluster shape, writes
`okdctl.yaml` and `okdctl.env`, then deploys. A deploy runs three phases:

1. **setup** installs the tool trio (`oc`, `openshift-install`,
   `terraform`) and host packages, renders manifests and custom CoreOS
   ISOs, and configures HAProxy, DNS, and the firewall on the bastion.
2. **install** brings the VMs up via Terraform, then waits for bootstrap
   to complete and CSRs to be approved.
3. **post-install** removes the bootstrap node, migrates ingress to
   LoadBalancer IPs when available, and installs any addons you picked.

Each phase is a sequence of steps. A step failure stops the run
(completed steps are not rolled back), prints a failure summary, and
leaves a deploy-state marker naming the active phase. Re-running `deploy`
resumes from that phase — once install has begun, setup is skipped so
cluster credentials are never regenerated under live VMs — and steps
whose work already exists are skipped. To tear down instead: after a
setup failure `okdctl cleanup` removes local files (no VMs exist yet);
once install has started, `okdctl destroy` removes the provisioned
resources.

## Commands

```
okdctl addon            manage cluster addons
okdctl cleanup          remove OKD cluster artifacts without destroying infrastructure
okdctl cluster          cluster-wide lifecycle operations
okdctl completion       generate shell completion script
okdctl config           inspect okdctl configuration
okdctl debug-bundle     collect a support bundle for troubleshooting
okdctl deploy           deploy a Kubernetes cluster
okdctl describe         show details for a cluster node or addon
okdctl destroy          destroy a Kubernetes cluster
okdctl doctor           check that your environment is ready to deploy a cluster
okdctl kubeconfig       print or export the cluster kubeconfig
okdctl node             manage cluster node lifecycle
okdctl plan             preview infrastructure drift without applying changes
okdctl releases         query available OKD versions
okdctl status           print a post-deploy cluster summary
okdctl update-ingress   switch ingress DNS from HAProxy to LoadBalancer IPs
okdctl version          print version, git commit, build date
```

Full command reference: [`docs/cli/okdctl.md`](docs/cli/okdctl.md).
Exit codes and shell-script idioms: [`docs/cli/exit-codes.md`](docs/cli/exit-codes.md).
JSON output shapes: [`docs/cli/json-schema.md`](docs/cli/json-schema.md).

### The working directory

One working directory is one cluster. okdctl writes everything next to
where you run it, and `deploy` works from any directory, including an
empty one. The first run:

- materializes the Terraform sources embedded in the binary into
  `infrastructure/terraform/` (write-once: existing files, such as a
  source checkout or hand-edited HCL, are never overwritten)
- launches the wizard
- writes `okdctl.yaml`, plus an `okdctl.env` file next to it for Proxmox
  credentials

The deploy itself adds an `okd-install/` work directory and Terraform
state under `infrastructure/terraform/environments/`.

`deploy`, `destroy`, and `cleanup` append their full log to `okdctl.log`
next to the config, with credentials redacted and one `run_id` per
invocation. A failed deploy stays diagnosable after the scrollback is
gone: `okdctl debug-bundle` picks the log up automatically, and
`--log-file` redirects it.

Later runs reuse the existing config. The commands that operate on an
existing cluster (`status`, `destroy`, `kubeconfig`, `debug-bundle`) must
run from the same directory. For multiple clusters, use one directory per
cluster; `--config other.yaml` selects an alternate config within one.

If the work directory holds a completed cluster, run `okdctl destroy`
before deploying again. `--fresh` force-wipes the directory without
destroying anything, and the credentials in it are lost.

The phase internals, the addon system, and the wizard architecture live
in [`docs/architecture/`](docs/architecture/). Per-addon reference:
[Flux](docs/addons/flux.md),
[external secret store](docs/addons/secretstore.md).

### Shell completion

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

## Configuration

Use the wizard. If you'd rather edit YAML directly, reference configs live
in [`configs/examples/`](configs/examples/):

- `minimal.yaml` — 1 control-plane node, 0 workers (single-node cluster)
- `production.yaml` — 3 control-plane nodes, 5 workers
- `media-server.yaml` — homelab setup with storage-heavy workers

The Proxmox credentials live in the `okdctl.env` file next to the
config, never in the YAML. Env vars: `PROXMOX_VE_ENDPOINT`,
`PROXMOX_VE_USERNAME`, `PROXMOX_VE_PASSWORD` (or
`PROXMOX_VE_API_TOKEN`).

### OKD pull secret

No Red Hat account needed. The dummy pull secret from
[openshift/okd](https://github.com/openshift/okd) works, since
`openshift-install` only schema-validates it at prompt time:

```json
{"auths":{"fake":{"auth":"aWQ6cGFzcwo="}}}
```

Save it as `~/pull-secret.json` (the wizard's default). Bring your own for
a private registry. Note that using a real `console.redhat.com` secret
with OKD may violate Red Hat's subscription terms; see
[okd-project/okd#1930](https://github.com/okd-project/okd/discussions/1930).

### Subscription-gated defaults

Stock OKD ships two things that cannot work without a Red Hat
subscription: the `redhat-operators`, `certified-operators`, and
`redhat-marketplace` OperatorHub catalogs, whose index images can never be
pulled, and a permanent `InsightsDisabled` alert, since the Insights
operator needs a `console.redhat.com` token no OKD install has (see
[okd-project/okd#2058](https://github.com/okd-project/okd/discussions/2058)).
`okdctl deploy` disables both during post-install. Pass
`--keep-redhat-catalogs` to leave them as OKD ships them.

## Troubleshooting

Run `okdctl doctor` first. It catches most common failures, and its
output belongs in bug reports.

- **Bootstrap VM never comes up.** Networking. The ignition URL must be
  reachable from the node network
  (`https://<ignition_server_ip>/ignition/<role>.ign`, port 443). Doctor
  probes this.
- **`dnsmasq` fails on port 53.** `systemd-resolved` has it. Set
  `DNSStubListener=no` in `/etc/systemd/resolved.conf`, restart
  `systemd-resolved`, then retry `okdctl deploy`.
- **`oc` not found mid-setup.** okdctl installs it into `/usr/local/bin`,
  which isn't on `$PATH` in the current shell until you re-source your rc.
- **Terraform destroy hangs.** The Proxmox API drops long-running destroy
  requests under load. Re-run `okdctl destroy`; state is preserved.
- **CSR approval fails on clock skew.** A Proxmox pause/resume can jump a
  guest's clock by minutes, which fails etcd elections and gets certs
  refused as "not yet valid". okdctl ships a chrony MachineConfig on every
  master and worker, pointed at the bastion, that steps the clock instead
  of slewing (`networking.ntp_server` overrides the source). Skew should
  self-heal within a few minutes of the node coming up. If it doesn't,
  check that the node can reach the configured NTP source.

## Uninstall

```sh
okdctl destroy                                                       # tear down the cluster
rm -rf okd-install infrastructure okdctl.yaml okdctl.env okdctl.log  # residual state
sudo rm /usr/local/bin/okdctl                                        # or: apt/dnf remove okdctl
```

`destroy` tears down the Terraform-provisioned VMs, removes the dnsmasq
drop-in, the HAProxy config and its backups, and the firewall rules okdctl
added, then uninstalls what setup installed: the `haproxy`, `dnsmasq`,
`httpd`, `coreos-installer`, and `terraform` packages, and the `oc`,
`openshift-install`, and `kubectl` binaries in the bin dir. The master
VMs' `prevent_destroy` guard is lifted automatically for the confirmed
run via a transient override that is removed when the destroy finishes.

## Verifying a release

Every tagged release ships:

- `okdctl_<version>_linux_<arch>.tar.gz` — binary archive (amd64 + arm64)
- `SHA256SUMS`, `SHA256SUMS.sig`, `SHA256SUMS.pem` — sigstore keyless signature
- `okdctl_<version>_linux_<arch>.sbom.json` — CycloneDX SBOM (binary archive)
- `okdctl_<version>_linux_<arch>.{deb,rpm}.sbom.json` — CycloneDX SBOMs (apt/rpm packages)
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
repository. No maintainer private keys, no trust in the release page
markup, no trust in any CDN between you and GitHub.

Each release also carries a GitHub artifact attestation (SLSA build
provenance recorded in the repository's attestations log):

```sh
gh attestation verify <file> --repo qxtaiba/okdctl
```

This checks the attestation against GitHub's Sigstore instance and confirms
the artifact was produced by a workflow in this repository. Requires the
[GitHub CLI](https://cli.github.com/) (`gh`).

The binaries are built with `-trimpath` and deterministic ldflags, so
`make build` from the tagged commit produces a byte-identical binary.
`sha256sum bin/okdctl` should match `SHA256SUMS`.

## Security considerations

Two known exposure windows are worth understanding before you deploy:

### Ignition pull-secret exposure window

During bootstrap (roughly 15–30 minutes), Apache on the bastion serves
`bootstrap.ign`, `master.ign`, and `worker.ign` over HTTPS on port 443.
These files embed the OKD pull-secret JSON in plain text.

okdctl binds Apache to `http_server.ignition_server_ip` (the bridge IP
FCOS nodes reference in their kargs ignition URL), not `0.0.0.0`, so
hosts off the machine network can't reach it. Each node ISO gets the
server's CA embedded via `coreos-installer iso customize --ignition-ca`,
so nodes verify the server before requesting files. The residual risk:
TLS authenticates the server, not the client. Any host that can reach the
bastion bridge IP on port 443 during bootstrap can retrieve the ignition
files and harvest the pull secret.

Mitigations:

- Isolate the bastion bridge network from untrusted hosts (VLAN, private
  bridge, or Proxmox SDN zone) before running `okdctl deploy`.
- Run `okdctl cleanup` after deploy completes. It removes the ignition
  files from the web root.

### SSH host-key trust on first run (TOFU window)

The first `okdctl deploy` scps CoreOS ISOs to the Proxmox host with
`-o StrictHostKeyChecking=accept-new`, trusting and pinning the Proxmox
host key without prior verification. Every later SSH/SCP call reuses that
cached key. A machine-in-the-middle on the bastion-to-Proxmox path during
that first SCP call can substitute an attacker key, which then stays
trusted for the life of the cluster.

Set `provider.proxmox.ssh_host_fingerprint` in `okdctl.yaml`
(`SHA256:<base64>`, from `ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub`
or the Proxmox UI) for deterministic verification; every later SSH/SCP
call then refuses on mismatch. Set
`provider.proxmox.require_pinned_fingerprint: true` to fail closed when
the pin is absent.

Other mitigations:

- Deploy from a bastion with a trusted L2 path to the Proxmox host (no
  NAT or L3 hop an attacker can sit on).
- Before the first deploy, SSH to the Proxmox host manually and verify its
  fingerprint out-of-band (Proxmox UI → Node → Shell → `ssh-keygen -lf
  /etc/ssh/ssh_host_ed25519_key.pub`). Once it's in `~/.ssh/known_hosts`,
  `accept-new` won't override it.

## Contributing

PRs welcome. You need Go 1.26 or newer; `go.mod` pins the toolchain, so
`go build` fetches the right one automatically.

```sh
make build        # builds ./bin/okdctl
make test         # unit tests with -race and coverage
make lint         # golangci-lint (installed on first run)
```

Install [lefthook](https://github.com/evilmartians/lefthook) and run
`lefthook install` to get the same checks as git hooks. The commit
messages follow conventional commits (`type(scope): description`,
lowercase, imperative). If you add or change CLI commands or flags, run
`make docs` and commit the regenerated pages under `docs/cli/`; CI fails
on drift.

For bugs, use the issue forms. They ask for the `okdctl version`,
`okdctl doctor`, and `okdctl debug-bundle` output I need to reproduce.

## License

Apache-2.0. Copyright 2026 Qutaiba Al-Nuaimy. See [LICENSE](LICENSE) and
[NOTICE](NOTICE).
