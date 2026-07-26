## okdctl cluster stop

Power off the cluster

### Synopsis

Cordon every node, then gracefully power off each worker (ascending)
followed by each master (ascending) through the Proxmox API.

Stop runs no drain: with the whole cluster stopping there is nowhere left to
reschedule a pod. The kubelet client-cert signer's remaining validity is
reported before the confirmation prompt, since it keeps expiring while the
cluster is stopped. Restart with 'okdctl cluster start'.

Stop refuses to run while a marker from any other in-flight node op is
recorded, since stop is not resumable and would otherwise overwrite that op's
resume trail. --acknowledge-interrupted-op overrides the marker and proceeds.

With ha_enabled set, masters are also managed by the Proxmox HA manager,
which may counteract an out-of-band shutdown (its request-state still says
started). Stop warns and proceeds; verify the power state afterwards, or set
the HA request-state to stopped via pvesh first.

```
okdctl cluster stop [flags]
```

### Examples

```
  okdctl cluster stop --yes --confirm-cluster grappleberry
  okdctl cluster stop --dry-run
```

### Options

```
      --acknowledge-interrupted-op   override a stranded marker left by an unrelated op and proceed fresh
      --confirm-cluster string       required with --yes; must equal the config cluster name
      --dry-run                      print the shutdown plan without powering anything off
  -h, --help                         help for stop
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

