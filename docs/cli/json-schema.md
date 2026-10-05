# okdctl JSON output schema

The `okdctl status --output=json` command emits machine-readable output
suitable for piping into `jq`, parsers, or higher-level automation. It is
the only JSON-producing command; this page documents its stable shape so
that consumers can pin against a known contract.

> **Stability:** the field names below are stable across patch and minor
> releases. New fields may be added (consumers must tolerate unknown keys);
> existing fields are not renamed or removed without a major bump.

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

## Conventions

- All boolean fields are `true` / `false`, not `"true"` / `"false"`.
- `null` is never emitted — fields that are absent are omitted entirely.
- `okdctl status` sets exit code `0` even when the cluster state is degraded;
  consumers of its JSON output determine state from payload fields, not from
  the exit code. See [exit-codes.md](exit-codes.md) for the full code taxonomy.
- Output is pretty-printed (`SetIndent("", "  ")`) for readability when piped
  to a file. Scripts that need compact JSON should pipe through `jq -c`.
