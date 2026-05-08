# okdctl JSON output schema

`okdctl` commands that accept `--output=json` emit machine-readable
output suitable for piping into `jq`, parsers, or higher-level automation.
This page documents the stable shape of every JSON-producing command so that
consumers can pin against a known contract.

> **Stability:** the field names below are stable across patch and minor
> releases. New fields may be added (consumers must tolerate unknown keys);
> existing fields are not renamed or removed without a major bump.

## `okdctl status --output=json`

Top-level cluster snapshot.

```json
{
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
| `api_reachable` | bool | `true` when `kube-apiserver /healthz` returns 200 |
| `nodes[].name` | string | node name from `kubectl get nodes` |
| `nodes[].role` | string | `master`, `worker`, or `unknown` |
| `nodes[].ready` | bool | node's `Ready` condition is `True` |
| `degraded_operators` | int | cluster-operators with `Degraded=True` |
| `addons[].name` | string | registered addon name |
| `addons[].healthy` | bool | `true` when verify returned no error |
| `addons[].error` | string | present only when `healthy=false` |

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

## Conventions

- All timestamps use RFC 3339 with a trailing `Z` for UTC.
- All boolean fields are `true` / `false`, not `"true"` / `"false"`.
- `null` is never emitted — fields that are absent are omitted entirely.
- `okdctl` sets exit code `0` on JSON success even when the underlying state
  is degraded; consumers determine state from payload fields, not from the
  exit code.
- Output is pretty-printed (`SetIndent("", "  ")`) for readability when piped
  to a file. Scripts that need compact JSON should pipe through `jq -c`.
