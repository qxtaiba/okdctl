# okdctl

[![CI](https://github.com/qxtaiba/okdctl/actions/workflows/ci.yml/badge.svg?branch=develop)](https://github.com/qxtaiba/okdctl/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

okdctl is a lifecycle tool for one OKD cluster on Proxmox VE: define the
cluster in one file, bring it up, change it, and take it down again, from a
single binary with no host of its own.

## Where the project stands

The installer is being rebuilt on the OKD agent-based installer with a
direct Proxmox VE driver. The install step itself is slice 3 of that
rebuild and has not landed yet. This binary defines a cluster and checks
the host it will land on; it does not install one.

The workflow today, from an empty directory:

1. `okdctl init` opens the configuration wizard and writes `okdctl.yaml`.
   The Proxmox API token never enters that file. The wizard writes it to
   `okdctl.env` beside it, mode 0600, and `PROXMOX_VE_API_TOKEN` or
   `PROXMOX_VE_INSECURE` in the environment overrides what the file holds.
   `okdctl init --minimal` writes the three-node starter file without the
   wizard. The wizard's review screen ends on "save and exit".
2. `okdctl config validate` checks the file against the schema and prints
   the node table it produces: name, role, VMID, MAC, address, host, CPU,
   memory and disk sizes for every control-plane and worker node. The
   MACs are a preview until the install mints the infra-ID. `okdctl config
   show` prints the file as the loader read it.
3. `okdctl doctor` runs the Proxmox preflight the installer relies on: API
   token privileges, PVE version, CPU flags, free memory per host, VMID and
   address collisions, storage content types and the bridge on every target
   node. That command is read-only. `--output json` prints the findings as
   an array.

`okdctl releases list` and `okdctl releases show` browse the OKD release
catalogue. `okdctl kubeconfig` prints an installed cluster's kubeconfig
from the workspace; nothing writes that file until the install step lands.

The configuration schema is v3 only. The loader refuses a v2 file and
points at `docs/migration.md`, which maps every v2 key to its v3 home.
Two complete examples live under `configs/examples/`; both pass
`okdctl config validate`.

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

- `cmd/okdctl` is the entry point.
- `internal/cli` is the cobra command tree.
- `internal/config` is the v3 configuration schema, its loader and the
  validators.
- `internal/model` derives every node identity (name, VMID, MAC, address,
  host) from the configuration; nothing else computes one.
- `internal/credentials` reads and writes the `okdctl.env` sidecar and
  merges the environment over it.
- `internal/workspace` owns the on-disk layout under the directory that
  holds `okdctl.yaml`.
- `internal/proxmox` is the Proxmox VE driver behind the `Driver`
  interface, with the preflight `doctor` runs.
- `internal/tui` is the wizard framework, its steps and the terminal
  styling.
- `internal/steps` is the step engine the install will be built from.
  Nothing in the binary runs it yet.

See `docs/architecture/wizard.md` for the wizard's design.

## License

Apache 2.0. See [LICENSE](LICENSE).
