# okdctl JSON output schema

`okdctl` commands that accept `--format=json` emit machine-readable
output suitable for piping into `jq`, parsers, or higher-level automation.
This page documents the stable shape of every JSON-producing command so that
consumers can pin against a known contract.

> **Stability:** the field names below are stable across patch and minor
> releases. New fields may be added (consumers must tolerate unknown keys);
> existing fields are not renamed or removed without a major bump.

## `okdctl status --format=json`

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

## `okdctl releases list --format=json`

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

## `okdctl releases show <version> --format=json`

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

## `okdctl describe node <name> --format=json`

```json
{
  "name": "master-0",
  "role": "master",
  "ready": true
}
```

## `okdctl describe addon <name> --format=json`

```json
{
  "name": "flux",
  "display-name": "Flux GitOps",
  "description": "GitOps toolkit for declarative cluster reconciliation",
  "category": "gitops",
  "health": "healthy"
}
```

> Note: `display-name` uses a hyphen for historical reasons. Other fields in
> this schema use snake_case; consumers piping between commands may need to
> normalize.

## Conventions

- All timestamps use RFC 3339 with a trailing `Z` for UTC.
- All boolean fields are `true` / `false`, not `"true"` / `"false"`.
- `null` is never emitted — fields that are absent are omitted entirely.
- `okdctl` sets exit code `0` on JSON success even when the underlying state
  is degraded; consumers determine state from payload fields, not from the
  exit code.
- Output is pretty-printed (`SetIndent("", "  ")`) for readability when piped
  to a file. Scripts that need compact JSON should pipe through `jq -c`.
