## okdctl cluster start

Power on the cluster

### Synopsis

Power on every master as one batch, then every worker, then wait for
every node to report Ready — approving pending kubelet CSRs on each poll so a
cluster restarted after certificate rotation rejoins unattended — and finally
uncordon every node.

Node enumeration is config-driven (cfg.Topology counts) rather than the
Kubernetes API: the API is hosted by the very VMs start has not powered on
yet.

Start refuses to run while a marker from any other in-flight node op is
recorded, since start is not resumable and would otherwise overwrite that
op's resume trail. --acknowledge-interrupted-op overrides the marker and
proceeds.

```
okdctl cluster start [flags]
```

### Examples

```
  okdctl cluster start --yes --confirm-cluster grappleberry
  okdctl cluster start --dry-run
```

### Options

```
      --acknowledge-interrupted-op   override a stranded marker left by an unrelated op and proceed fresh
      --confirm-cluster string       required with --yes; must equal the config cluster name
      --dry-run                      print the power-on plan without powering anything on
  -h, --help                         help for start
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

* [okdctl cluster](okdctl_cluster.md)	 - Manage cluster-wide lifecycle operations

