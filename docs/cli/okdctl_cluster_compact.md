## okdctl cluster compact

Consolidate the cluster onto its control plane

### Synopsis

Make the control plane schedulable, apply the compact IngressController,
then remove workers top-down — optionally growing masters interleaved so a
freed worker always precedes a grown master (memory-budget ordering).

This is a thin orchestrator over 'node remove' and 'node resize'; it adds no
new mutation mechanics and inherits their guards (storage, ingress, etcd).

An interrupted compaction resumes automatically: each inner worker removal or
master resize carries its own op marker, so re-running 'okdctl cluster compact'
picks up at the node/step that was in flight.
--acknowledge-interrupted-op overrides a marker left by an unrelated op
instead of refusing.

```
okdctl cluster compact [flags]
```

### Examples

```
  okdctl cluster compact --yes --confirm-cluster grappleberry
  okdctl cluster compact --grow-master-memory-mb 24576 --dry-run
```

### Options

```
      --acknowledge-interrupted-op   override a stranded marker left by an unrelated op and proceed fresh
      --confirm-cluster string       required with --yes; must equal the config cluster name
      --dry-run                      print the compaction plan without mutating anything
      --force-storage                allow worker removal even when workers hold rook-ceph OSDs
      --grow-master-memory-mb int    resize each master to this memory (MiB) as workers are removed (0 leaves masters unchanged)
  -h, --help                         help for compact
      --ingress-replicas int         compact IngressController replica count (default 2)
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

* [okdctl cluster](okdctl_cluster.md)	 - Cluster-wide lifecycle operations

