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
| `phase` | string | lifecycle state: `Pending`, `Installing`, `Running`, `Degraded`, `Failed`, or `Unknown`; always present |
| `api_reachable` | bool | `true` when `kube-apiserver /healthz` returns 200 |
| `nodes[].name` | string | node name from `kubectl get nodes` |
| `nodes[].role` | string | `master`, `worker`, or `unknown` |
| `nodes[].ready` | bool | node's `Ready` condition is `True` |
| `nodes[].status` | string | `Ready`, `NotReady`, or `Unknown`; the CLI only ever emits `Ready`/`NotReady` today |
| `nodes[].version` | string | reserved for a future kubelet-version projection; not populated by any command today, so never emitted |
| `nodes[].internal_ip` | string | reserved for a future internal-IP projection; not populated by any command today, so never emitted |
| `nodes[].conditions` | array | reserved for a future per-node condition list; not populated by any command today, so never emitted |
| `degraded_operators` | int | cluster-operators with `Degraded=True` |
| `addons[].name` | string | registered addon name |
| `addons[].healthy` | bool | `true` when verify returned no error |
| `addons[].error` | string | present only when `healthy=false` |
| `addons` | array | addon health snapshots; present when non-empty |

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

Preflight check envelope. Exit code follows the documented contract (0 = no
failures, 2 = one or more failing checks) regardless of format.

```json
{
  "checks": [
    {"name": "host os", "severity": "ok", "detail": "fedora 41 (rhel family)"},
    {"name": "tools and packages/oc", "severity": "warn", "detail": "will be downloaded during setup"},
    {"name": "host ports", "severity": "fail", "detail": "in use: 80, 443 (stop the conflicting service before deploy)"}
  ],
  "failed": 1,
  "warned": 1
}
```

| Field | Type | Notes |
|---|---|---|
| `checks[].name` | string | check name; multi-item checks use `<check>/<item>` notation |
| `checks[].severity` | string | `ok`, `warn`, or `fail` |
| `checks[].detail` | string | human-readable result text; omitted when empty |
| `failed` | int | number of checks with severity `fail` |
| `warned` | int | number of checks with severity `warn` |

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
  `doctor --output=json` exits `2` when any check is `[fail]`;
  `addon verify --output=json` exits `4` when any probe fails.
  See [exit-codes.md](exit-codes.md) for the full code taxonomy.
- Output is pretty-printed (`SetIndent("", "  ")`) for readability when piped
  to a file. Scripts that need compact JSON should pipe through `jq -c`.
