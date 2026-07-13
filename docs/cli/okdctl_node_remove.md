## okdctl node remove

Remove a worker node from the cluster

### Synopsis

Cordon and drain a worker, destroy its VM via a plan-gated targeted
terraform apply, then delete its Kubernetes Node object.

Only the highest-numbered worker is removable: terraform count reduction
destroys the last instance, so workers must be removed top-down. Guards refuse
removal when the worker holds rook-ceph OSDs (data loss) or when router pods
run on workers with a non-schedulable control plane (ingress outage).

```
okdctl node remove <name> [flags]
```

### Examples

```
  okdctl node remove worker2 --yes --confirm-cluster grappleberry
  okdctl node remove worker2 --dry-run
```

### Options

```
      --confirm-cluster string   required with --yes; must equal the config cluster name
      --drain-timeout string     per-node drain timeout (default "10m")
      --dry-run                  run guards and the plan gate without mutating anything
      --force-storage            allow removal even when the worker holds rook-ceph OSDs (destroys their data disk)
  -h, --help                     help for remove
      --skip-drain               skip cordon/drain (assumes the node is already evacuated)
  -y, --yes                      skip confirmation prompt
```

### Options inherited from parent commands

```
  -c, --config string       configuration file (default "okdctl.yaml")
      --log-file string     write log output to this file in addition to stderr (replaces the default okdctl.log sink of deploy/destroy/cleanup)
      --log-format string   log output format: text (TTY default) | json (auto-selected when stderr is piped)
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
  -q, --quiet               suppress info/warn logs (alias for --log-level=error)
  -v, --verbose             enable debug logging (alias for --log-level=debug)
```

### SEE ALSO

* [okdctl node](okdctl_node.md)	 - Manage cluster node lifecycle

