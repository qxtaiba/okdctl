## okdctl node snapshot create

Snapshot a node's disks

### Synopsis

Snapshot target's disks via pvesh. A Ready node is cordoned and drained
first unless --skip-drain is set; a NotReady node is snapshotted directly,
since a drain would only spin with nowhere to reschedule its pods.

Crash-consistent only: qemu-guest-agent is disabled fleet-wide, so this is
equivalent to the VM losing power, not a clean shutdown.

Create refuses to run while a marker from any other in-flight node op is
recorded, since snapshot is not resumable and would otherwise overwrite that
op's resume trail. --acknowledge-interrupted-op overrides the marker and
proceeds.

```
okdctl node snapshot create <node> [flags]
```

### Examples

```
  okdctl node snapshot create worker0 --yes --confirm-cluster grappleberry
  okdctl node snapshot create worker0 --name pre-upgrade --dry-run
```

### Options

```
      --acknowledge-interrupted-op   override a stranded marker left by an unrelated op and proceed fresh
      --confirm-cluster string       required with --yes; must equal the config cluster name
      --description string           optional snapshot description (single token: no spaces; use dashes or underscores)
      --drain-timeout string         drain timeout when the node is cordoned first (default "10m")
      --dry-run                      report what would happen without creating a snapshot
  -h, --help                         help for create
      --name string                  snapshot name (default okdctl-<UTC timestamp>)
      --skip-drain                   skip cordon/drain before snapshotting a Ready node
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

* [okdctl node snapshot](okdctl_node_snapshot.md)	 - Manual, single-node Proxmox VM snapshots

