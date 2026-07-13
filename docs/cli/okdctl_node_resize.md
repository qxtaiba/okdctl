## okdctl node resize

Resize node CPU/memory per role, rolled out one node at a time

### Synopsis

Change per-role node resources and roll the change out one node at a
time. Masters are etcd-health-gated before and after every node and applied
with an in-place-update plan gate (a VM replace is refused). Workers roll
without the etcd gate.

Sizing is per-role: the role's memory/cpu knob is updated in config and tfvars,
but each targeted apply mutates only the current node; other same-role nodes
pick up the pending change on the next full deploy.

At least one of --memory-mb or --cpu is required; an omitted dimension keeps
the role's current value.

```
okdctl node resize (masters|workers|<name>) [--memory-mb N] [--cpu N] [flags]
```

### Examples

```
  okdctl node resize masters --memory-mb 24576 --yes --confirm-cluster grappleberry
  okdctl node resize workers --memory-mb 16384 --dry-run
  okdctl node resize workers --cpu 8 --yes --confirm-cluster grappleberry
```

### Options

```
      --confirm-cluster string   required with --yes; must equal the config cluster name
      --cpu int                  new per-node cpu cores (0 keeps current)
      --dry-run                  run gates and the plan gate without mutating anything
  -h, --help                     help for resize
      --memory-mb int            new per-node memory in MiB (0 keeps current)
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

