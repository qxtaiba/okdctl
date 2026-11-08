# okdctl

[![CI](https://github.com/qxtaiba/okdctl/actions/workflows/ci.yml/badge.svg?branch=develop)](https://github.com/qxtaiba/okdctl/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

okdctl is a lifecycle tool for one OKD cluster on Proxmox VE: define the
cluster in one file, bring it up, change it, and take it down again, from a
single binary with no host of its own.

## Where the project stands

The installer is being rebuilt on the OKD agent-based installer. The
previous design (user-provisioned infrastructure with a bastion host running
HAProxy, dnsmasq and an ignition server, driven through Terraform) has been
removed from this branch; its history is the `develop` commits before the
demolition change. Until the new install path lands, this binary does three
things:

- `okdctl deploy` opens the configuration wizard and saves `okdctl.yaml`
  (with credentials in `okdctl.env`). Choosing "deploy now" saves the file
  and prints a note that installation returns in a later release.
- `okdctl config show` and `okdctl config validate` read and check that
  file.
- `okdctl releases list` and `okdctl releases show` browse the OKD release
  catalogue; `okdctl kubeconfig` prints an existing cluster's kubeconfig.

## What it isn't

- Not a managed service and not a wrapper around a hosted installer.
- Not k3s or microk8s. This is upstream OKD with the full operator surface.
- Not multi-cluster or multi-tenant; one cluster per `okdctl.yaml`.
- Not a stable product yet. The config schema changes between minors; pin
  your version.

okdctl collects no telemetry and no analytics.

## Building

```sh
make build        # ./bin/okdctl
make test
make lint         # the CI-pinned golangci-lint
```

Go version and toolchain come from `go.mod`.

## Layout

- `cmd/okdctl` — entry point.
- `internal/cli` — the cobra command tree.
- `internal/config` — the configuration schema, loader and validators.
- `internal/tui` — the wizard framework, its steps and the terminal styling.
- `internal/cluster` — the `oc`/`kubectl` client, CSR policy and health gates.
- `internal/steps` — the step engine phases are built from.
- `internal/infrastructure/proxmox` — the Proxmox API probe and power calls.

See `docs/architecture/wizard.md` for the wizard's design.

## License

Apache 2.0. See [LICENSE](LICENSE).
