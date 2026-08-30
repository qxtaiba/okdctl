# okdctl JSON output schema

`okdctl` commands that accept `--output=json` emit machine-readable
output suitable for piping into `jq`, parsers, or higher-level automation.
This page documents the stable shape of every JSON-producing command so that
consumers can pin against a known contract.

> **Stability:** the field names below are stable across patch and minor
> releases. New fields may be added (consumers must tolerate unknown keys);
> existing fields are not renamed or removed without a major bump.

## `okdctl config show --output=json`

Redacted configuration. All string fields whose JSON tag name matches the
secret-key denylist are replaced with `"***"`. Fields tagged `json:"-"`
(Password, APIToken, Username) are omitted entirely.

The exact shape mirrors the `config.Config` Go struct and its nested types.
A minimal Proxmox-backed example:

```json
{
  "provider": {
    "type": "proxmox",
    "proxmox": {
      "host": "pve.example",
      "node": "pve",
      "token_id": "***"
    }
  }
}
```

Without `--output`, the command emits YAML (existing behavior, unchanged).

## `okdctl status --output=json`

Top-level cluster snapshot.

```json
{
  "phase": "Running",
  "api_reachable": true,
  "nodes": [
    {"name": "master-0", "role": "master", "ready": true},
    {"name": "worker-0", "role": "worker", "ready": true}
  ],
  "degraded_operators": 0,
  "addons": [
    {"name": "flux", "healthy": true},
    {"name": "secretstore", "healthy": false, "error": "ConditionStatus: False"}
  ]
}
```

| Field | Type | Notes |
|---|---|---|
| `phase` | string | lifecycle state: `Pending`, `Installing`, `Running`, `Degraded`, or `Unknown`; always present |
| `api_reachable` | bool | `true` when `kube-apiserver /healthz` returns 200 |
| `nodes[].name` | string | node name from `kubectl get nodes` |
| `nodes[].role` | string | `master`, `worker`, or `unknown` |
| `nodes[].ready` | bool | node's `Ready` condition is `True` |
| `nodes[].status` | string | `Ready`, `NotReady`, or `Unknown` |
| `degraded_operators` | int | cluster-operators with `Degraded=True` |
| `addons[].name` | string | registered addon name |
| `addons[].healthy` | bool | `true` when verify returned no error |
| `addons[].error` | string | present only when `healthy=false` |
| `addons` | array | addon health snapshots; present when non-empty |

## `okdctl node list --output=json`

Flat array, one entry per cluster node.

```json
[
  {"name": "master-0", "role": "master", "ready": true, "tf_index": 0, "drift": "none"},
  {
    "name": "worker-2", "role": "worker", "ready": false, "tf_index": 2,
    "drift": "pending", "drift_detail": "config 16384MiB/4cpu/100GiB vs tfvars 8192MiB/4cpu/50GiB",
    "in_flight_op": "resize (tf-apply)"
  }
]
```

| Field | Type | Notes |
|---|---|---|
| `[].name` | string | node name from `kubectl get nodes` |
| `[].role` | string | `master`, `worker`, or `unknown` |
| `[].ready` | bool | node's `Ready` condition is `True` |
| `[].tf_index` | int | trailing numeric suffix of the node name (`worker2` → `2`), mapping the node to its terraform count index; omitted when the name has no numeric suffix |
| `[].drift` | string | `none`, `pending`, or `unknown` — compares the config file's per-role cpu/memory/os-disk size to what was last rendered into terraform.tfvars. This is **not** a live VM query (okdctl fetches no per-guest Proxmox sizing anywhere today): `pending` means a sizing change is staged in the workspace, not that the node's guest has actually been resized. `unknown` means either terraform.tfvars has not been rendered yet, or the node's `role` is `unknown` (no config sizing exists to compare against) |
| `[].drift_detail` | string | present only when `drift=pending`; the compared config/tfvars values |
| `[].in_flight_op` | string | present only on the node targeted by an in-flight `remove`/`resize`/`compact` op's on-disk marker, formatted `"<op> (<step>)"` |

An op marker whose target matches no listed node — a `cluster stop`/`cluster
start` marker (its target is the cluster name, not a node) or a marker naming
a since-removed node — is **not** part of the JSON array. It surfaces only in
the text output as a trailing `in-flight op: …` note; scripted callers that
need it read the marker via `okdctl node list` text or the on-disk marker
directly.

## `okdctl node snapshot list <node> --output=json`

Flat array of one node's Proxmox VM snapshots, in the order pvesh reports
them (not guaranteed chronological). The synthetic `current` entry Proxmox
uses to anchor its snapshot tree is filtered out.

```json
[
  {
    "name": "pre-upgrade", "description": "before upgrading to 4.21",
    "snap_time": "2026-04-12T15:00:00Z"
  },
  {"name": "baseline", "parent": "pre-upgrade"}
]
```

| Field | Type | Notes |
|---|---|---|
| `name` | string | snapshot name (pve-configid grammar: letter-led, `[A-Za-z0-9_-]`, ≤40 chars) |
| `description` | string | optional free-text note set at `snapshot create --description`; omitted when empty |
| `snap_time` | RFC3339 string | when the snapshot was taken; omitted when Proxmox reports no timestamp |
| `parent` | string | the snapshot this one was taken on top of, if any; omitted for a root snapshot |

## `okdctl plan --output=json`

Read-only terraform-plan drift preview. Exit code is `0` when `drift` is
`false`, `7` when `drift` is `true` — see [exit-codes.md](exit-codes.md).

```json
{
  "drift": true,
  "changes": [
    {"address": "module.okd_cluster.proxmox_virtual_environment_vm.worker[2]", "action": "update"}
  ]
}
```

| Field | Type | Notes |
|---|---|---|
| `drift` | bool | `true` when `changes` is non-empty |
| `changes[].address` | string | terraform resource address |
| `changes[].action` | string | `create`, `update`, `delete`, `replace`, or `unknown` — see `terraform.PlanAction` |
| `changes` | array | empty array (`[]`), never omitted, when the plan is clean |

`deploy --dry-run` renders the same change list (via the shared
`render.PlanPreview`) but has no `--output=json` mode of its own and keeps
exiting `0` regardless of drift — only `okdctl plan` carries the drift exit
code.

## `okdctl releases list --output=json`

Flat array of OKD releases (newest first). The CLI's human-readable mode
groups by minor series, but the JSON mode intentionally flattens for simple
`jq`-style filtering.

```json
[
  {
    "version": "4.21.3",
    "tag": "4.21.3-okd-scos.0",
    "release_date": "2026-04-12T15:00:00Z",
    "stable": true,
    "latest": true,
    "release_type": "stable"
  },
  {
    "version": "4.21.2",
    "tag": "4.21.2-okd-scos.0",
    "release_date": "2026-03-08T11:00:00Z",
    "stable": true,
    "latest": false,
    "release_type": "stable"
  }
]
```

| Field | Type | Notes |
|---|---|---|
| `version` | string | semver-shaped release version |
| `tag` | string | upstream Git tag |
| `release_date` | RFC3339 string | upstream `published_at` |
| `stable` | bool | `true` for GA releases, `false` for previews |
| `latest` | bool | `true` only for the newest stable in its minor series |
| `release_type` | string | `stable`, `prerelease`, or similar display classification |

When invoked with `--channel stable` (the default), only `stable=true`
releases appear. Use `--channel all` to include prereleases.

## `okdctl releases show <version> --output=json`

Single release detail — same `OKDVersion` shape as an element of
`releases list`.

```json
{
  "version": "4.21.3",
  "tag": "4.21.3-okd-scos.0",
  "release_date": "2026-04-12T15:00:00Z",
  "stable": true,
  "latest": true,
  "release_type": "stable"
}
```

## `okdctl describe node <name> --output=json`

```json
{
  "name": "master-0",
  "role": "master",
  "ready": true
}
```

## `okdctl describe addon <name> --output=json`

```json
{
  "name": "flux",
  "display_name": "Flux GitOps",
  "description": "GitOps toolkit for declarative cluster reconciliation",
  "category": "gitops",
  "health": "healthy"
}
```

## `okdctl addon list --output=json`

Flat array of registered addons with their config-file state.

```json
[
  {"name": "flux", "display_name": "Flux GitOps", "deps": [], "in_config": true},
  {"name": "secretstore", "display_name": "External Secrets Operator", "deps": ["flux"], "in_config": false}
]
```

| Field | Type | Notes |
|---|---|---|
| `name` | string | addon registration key |
| `display_name` | string | human-readable addon name |
| `deps` | string array | names of addons that must be installed first; `[]` when none |
| `in_config` | bool | `true` when the addon's `enabled` flag is set in the configuration file |

## `okdctl addon verify --output=json`

Flat array of health results for all enabled addons. Empty array (`[]`) when
no addons are enabled.

```json
[
  {"name": "flux", "healthy": true},
  {"name": "secretstore", "healthy": false, "error": "ConditionStatus: False"}
]
```

| Field | Type | Notes |
|---|---|---|
| `name` | string | addon registration key |
| `healthy` | bool | `true` when verify returned no error |
| `error` | string | present only when `healthy=false`; omitted otherwise |

Identical shape to the `addons[]` entries in `okdctl status --output=json`.

## `okdctl doctor --output=json`

Preflight check envelope. Exit code follows the documented contract (0 =
every check passes, 6 = one or more checks warn but none fail, 2 = one or
more failing checks) regardless of format.

```json
{
  "checks": [
    {"name": "host os", "severity": "ok", "detail": "fedora 41 (rhel family)"},
    {"name": "tools and packages/oc", "severity": "warn", "detail": "will be downloaded during setup"},
    {"name": "host ports", "severity": "fail", "detail": "in use: 80, 443 (stop the conflicting service before deploy)"},
    {"name": "cluster/nodes", "severity": "ok", "detail": "6 ready"},
    {"name": "cluster/cluster operators", "severity": "fail", "detail": "degraded: ingress"},
    {"name": "cluster/pending csrs", "severity": "warn", "detail": "2 awaiting approval"},
    {"name": "cluster/etcd", "severity": "ok", "detail": "healthy (3/3 pods ready)"},
    {"name": "cluster/signer expiry", "severity": "warn", "detail": "kube-apiserver-to-kubelet-signer expires in 21 days (2026-08-04) — rotate before kubelets fail"}
  ],
  "failed": 2,
  "warned": 3
}
```

| Field | Type | Notes |
|---|---|---|
| `checks[].name` | string | check name; multi-item checks use `<check>/<item>` notation |
| `checks[].severity` | string | `ok`, `warn`, or `fail` |
| `checks[].detail` | string | human-readable result text; omitted when empty |
| `failed` | int | number of checks with severity `fail` |
| `warned` | int | number of checks with severity `warn` |

The `cluster/*` entries are a day-2 section that appears **only when a deployed
cluster's kubeconfig is present** (located the same way `okdctl status` finds
it); pre-deploy runs omit them entirely, so their presence is not guaranteed.
The section reports `cluster/nodes` (NotReady = `fail`), `cluster/cluster
operators` (Degraded = `fail`, Progressing = `warn`), `cluster/pending csrs`
(any pending = `warn`), `cluster/etcd` (unhealthy = `fail`), and
`cluster/signer expiry` (expired = `fail`, under 30 days = `warn`). A
`cluster/csr recovery` entry is added only when pending CSRs coincide with
NotReady nodes. When the cluster API is unreachable a single `cluster/api`
`warn` entry replaces the section rather than five cascading errors. These
entries feed `failed`/`warned` and the exit code exactly like the host checks.

## `okdctl version --output=json`

Build identity for the running binary. Suitable for CI version pinning
and scripted comparisons.

```json
{
  "version": "0.8.1",
  "git_commit": "abc1234",
  "build_date": "2026-05-23T12:00:00Z",
  "go_version": "go1.25.0",
  "platform": "linux/amd64"
}
```

| Field | Type | Notes |
|---|---|---|
| `version` | string | semver build version injected by goreleaser |
| `git_commit` | string | short SHA of the commit the binary was built from |
| `build_date` | string | RFC3339 timestamp of the build, or `"unknown"` for local builds |
| `go_version` | string | Go toolchain version string (e.g. `go1.25.0`) |
| `platform` | string | `GOOS/GOARCH` of the build target |

## Conventions

- All timestamps use RFC 3339 with a trailing `Z` for UTC.
- All boolean fields are `true` / `false`, not `"true"` / `"false"`.
- `null` is never emitted — fields that are absent are omitted entirely.
- `okdctl status` sets exit code `0` even when the cluster state is degraded;
  consumers of its JSON output determine state from payload fields, not from
  the exit code. This guarantee applies to `status` only — other commands
  that emit JSON still exit non-zero on failure:
  `doctor --output=json` exits `6` when checks warn but none fail, and
  `2` when any check is `[fail]`;
  `addon verify --output=json` exits `4` when any probe fails;
  `plan --output=json` exits `7` when `drift` is `true`.
  See [exit-codes.md](exit-codes.md) for the full code taxonomy.
- Output is pretty-printed (`SetIndent("", "  ")`) for readability when piped
  to a file. Scripts that need compact JSON should pipe through `jq -c`.
