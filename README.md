# okdctl

[![CI](https://github.com/qxtaiba/okdctl/actions/workflows/ci.yml/badge.svg?branch=develop)](https://github.com/qxtaiba/okdctl/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/qxtaiba/okdctl?sort=semver)](https://github.com/qxtaiba/okdctl/releases)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

okdctl provisions OKD clusters on Proxmox VE from an interactive wizard.
I built it for the homelab operator with one or two Proxmox nodes who
wants a real Kubernetes cluster without hand-rolling Terraform, Ignition,
and bootstrap glue.

![okdctl deploy wizard](docs/assets/demo.gif)

## What it isn't

- Not a managed service, not a wrapper around the Red Hat Assisted Installer.
- Not k3s or microk8s — this is upstream OKD with the full operator surface.
- Not multi-cluster or multi-tenant; one cluster per `okdctl.yaml`.
- Not a production OKD distribution. Pre-1.0 — the config schema breaks
  between minors (every break is in the [CHANGELOG](CHANGELOG.md)); pin
  your version.

okdctl collects no telemetry and no analytics. The one request it makes on
its own behalf is an update check: release builds send a plain HTTPS GET to
`api.github.com` (`releases/latest`) — nothing beyond what any HTTP request
carries — at most once per 24 hours, cached in
`~/.cache/okdctl/update-check.json`. Set `OKDCTL_NO_UPDATE_CHECK=1` to turn
it off.

## Install

Linux only — the deploy phase shells out to `dnf`/`apt`, `firewall-cmd`,
`nmcli`, and `systemctl`. Install this on the bastion host you'll deploy
from.

```bash
curl -sSfL https://raw.githubusercontent.com/qxtaiba/okdctl/develop/scripts/install.sh | bash
```

`.deb`/`.rpm` packages, building from source, shell completion, and
release verification are covered in
[docs/verifying-releases.md](docs/verifying-releases.md) and
[docs/cli/completions.md](docs/cli/completions.md).

## Quick start

```sh
mkdir my-cluster && cd my-cluster
okdctl deploy
```

The wizard asks for Proxmox credentials and cluster shape, writes
`okdctl.yaml` and `okdctl.env`, then deploys. One directory is one
cluster; re-running `deploy` after an interruption resumes where it left
off — see [docs/architecture/workspace.md](docs/architecture/workspace.md)
for the full layout and [configs/examples/](configs/examples/) for
ready-made configs.

A deploy runs three phases: **setup** installs the tool trio (`oc`,
`openshift-install`, `terraform`) and host packages, renders manifests and
custom CoreOS ISOs, and configures HAProxy/DNS/firewall on the bastion;
**install** brings the VMs up via Terraform and waits for bootstrap and
CSR approval; **post-install** removes the bootstrap node, migrates
ingress to LoadBalancer IPs if available, and installs any addons you
picked. Each phase is a sequence of steps with rollback on failure. Phase
internals, the addon system, and the wizard live in
[docs/architecture/](docs/architecture/).

### OKD pull secret

No Red Hat account needed — the dummy pull secret from
[openshift/okd](https://github.com/openshift/okd) works, since
`openshift-install` only schema-validates it at prompt time:

```json
{"auths":{"fake":{"auth":"aWQ6cGFzcwo="}}}
```

Save it as `~/pull-secret.json` (the wizard's default). Bring your own for
a private registry; using a real `console.redhat.com` secret with OKD may
violate Red Hat's subscription terms — see
[okd-project/okd#1930](https://github.com/okd-project/okd/discussions/1930).

## Troubleshooting

Run `okdctl doctor` first — it catches most common failures. Specific
failure modes: [docs/troubleshooting.md](docs/troubleshooting.md).

## Uninstall

```sh
okdctl destroy                                                      # tear down the cluster
rm -rf okd-install infrastructure okdctl.yaml okdctl.env okdctl.log  # residual state
sudo rm /usr/local/bin/okdctl                                        # or: apt/dnf remove okdctl
```

## Contributing

PRs welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) for dev setup,
lint/coverage gates, and commit conventions. Run `make test && make lint`
before submitting. For bugs, use the issue forms — they ask for the
`okdctl version`, `okdctl doctor`, and `okdctl debug-bundle` output I
need to reproduce.

## License

Apache-2.0. Copyright 2026 Q Al Nuaimi. See [LICENSE](LICENSE) and
[NOTICE](NOTICE).
