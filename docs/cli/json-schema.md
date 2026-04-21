# okdctl JSON output schema

`okdctl` commands that accept `--format=json` (`-F json`) emit machine-readable
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
  "cluster_name": "homelab",
  "version": "4.21.0",
  "ready_nodes": 4,
  "total_nodes": 4,
  "degraded_operators": 0,
  "addons": [
    {"name": "flux", "healthy": true},
    {"name": "secretstore", "healthy": false, "error": "ConditionStatus: False"}
  ]
}
```

| Field | Type | Notes |
|---|---|---|
| `cluster_name` | string | from configuration |
| `version` | string | OKD release version reported by the cluster |
| `ready_nodes` | int | nodes whose `Ready` condition is `True` |
| `total_nodes` | int | total nodes in the cluster |
| `degraded_operators` | int | cluster-operators with `Degraded=True` |
| `addons[].name` | string | registered addon name |
| `addons[].healthy` | bool | true when verify returned no error |
| `addons[].error` | string | present only when `healthy=false` |

## `okdctl releases list --format=json`

Catalog of OKD releases grouped by minor series.

```json
{
  "series": [
    {
      "minor": "4.21",
      "latest": {"version": "4.21.3", "tag": "4.21.0-okd-scos.0", "stable": true, "release_date": "2026-04-12T15:00:00Z"},
      "versions": [
        {"version": "4.21.3", "tag": "4.21.3-okd-scos.0", "stable": true, "release_date": "2026-04-12T15:00:00Z", "latest": true},
        {"version": "4.21.2", "tag": "4.21.2-okd-scos.0", "stable": true, "release_date": "2026-03-08T11:00:00Z"}
      ]
    }
  ]
}
```

| Field | Type | Notes |
|---|---|---|
| `series[].minor` | string | major.minor identifier (e.g. `4.21`) |
| `series[].latest` | object | newest stable release within the series |
| `series[].versions[].version` | string | semver-shaped release version |
| `series[].versions[].tag` | string | upstream Git tag |
| `series[].versions[].stable` | bool | true for GA releases; false for previews |
| `series[].versions[].release_date` | RFC3339 string | upstream `published_at` |
| `series[].versions[].latest` | bool | present (and true) only on the newest stable in the series |

## `okdctl releases show <version> --format=json`

Single release detail.

```json
{
  "version": "4.21.3",
  "tag": "4.21.3-okd-scos.0",
  "stable": true,
  "release_date": "2026-04-12T15:00:00Z",
  "minor": "4.21",
  "short_version": "4.21"
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

## Conventions

- All timestamps use RFC 3339 with a trailing `Z` for UTC.
- All boolean fields are `true` / `false`, not `"true"` / `"false"`.
- `null` is never emitted — fields that are absent are omitted entirely.
- `okdctl` sets exit code `0` on JSON success even when the underlying state
  is degraded; consumers determine state from payload fields, not from the
  exit code.
- Output is pretty-printed (`SetIndent("", "  ")`) for readability when piped
  to a file. Scripts that need compact JSON should pipe through `jq -c`.
