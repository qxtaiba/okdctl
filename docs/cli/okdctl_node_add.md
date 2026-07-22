## okdctl node add

Add worker node(s) to the cluster

### Synopsis

Build and upload a per-node CoreOS ISO, revive the ignition HTTPS server for
the join window, apply a plan-gated targeted terraform create, and wait for
each new node to join and report Ready.

Only worker nodes can be added (master add/remove is not supported). --count
adds N workers in one batch, occupying the next N terraform count indices
after the persisted worker count. The ignition server is revived once for the
whole batch and torn down when the batch finishes, fails, or times out.

An interrupted add records an op marker and resumes automatically on the
next 'okdctl node add', skipping already-joined nodes and completed steps.
--acknowledge-interrupted-op overrides a marker left by a different op or
node instead of refusing.

```
okdctl node add --role worker [--count N] [flags]
```

### Examples

```
  okdctl node add --role worker --yes --confirm-cluster grappleberry
  okdctl node add --role worker --count 2 --dry-run
```

### Options

```
      --acknowledge-interrupted-op   override a stranded marker left by a different op or node and proceed fresh
      --confirm-cluster string       required with --yes; must equal the config cluster name
      --count int                    number of nodes to add in this batch (default 1)
      --dry-run                      run guards and the plan gate without mutating anything
  -h, --help                         help for add
      --role string                  node role to add (only worker is supported) (default "worker")
  -y, --yes                          skip confirmation prompt
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

