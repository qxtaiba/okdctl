## okdctl node snapshot

Manual, single-node Proxmox VM snapshots

### Synopsis

Take, list, roll back, and delete point-in-time Proxmox VM snapshots for
one node at a time, over pvesh via SSH.

This is a bounded safety net, not backup/DR: snapshots are manual (no
scheduling), single-node (no fleet-wide consistency across the cluster), and
short-lived (no retention policy — you are responsible for deleting what you
create). Every snapshot is crash-consistent only: qemu-guest-agent is
disabled fleet-wide, so a snapshot captures disk state as if the VM had lost
power, not as if it had been cleanly shut down.

### Options

```
  -h, --help   help for snapshot
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
* [okdctl node snapshot create](okdctl_node_snapshot_create.md)	 - Snapshot a node's disks
* [okdctl node snapshot delete](okdctl_node_snapshot_delete.md)	 - Delete a node's Proxmox snapshot
* [okdctl node snapshot list](okdctl_node_snapshot_list.md)	 - List a node's Proxmox snapshots
* [okdctl node snapshot rollback](okdctl_node_snapshot_rollback.md)	 - Roll back a node's disks to a prior snapshot

