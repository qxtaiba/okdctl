## okdctl node snapshot rollback

Roll back a node's disks to a prior snapshot

### Synopsis

Restore target's disks to name and power the VM back on — pvesh passes
-start 1 unconditionally, so a VM that was deliberately powered off comes
back up too.

A master rollback is quorum-sensitive: a crash-consistent snapshot can leave
etcd's Raft term or rook-ceph's OSD state stale relative to peers that kept
running, so the op refuses to start against an already-unhealthy quorum and
re-verifies health before the node is uncordoned. Any failure from the
cordon onward leaves the node cordoned; the error names the failing stage.

Rollback refuses to run while a marker from any other in-flight node op is
recorded, since snapshot is not resumable and would otherwise overwrite that
op's resume trail. --acknowledge-interrupted-op overrides the marker and
proceeds.

```
okdctl node snapshot rollback <node> <name> [flags]
```

### Examples

```
  okdctl node snapshot rollback worker0 pre-upgrade --yes --confirm-cluster grappleberry
  okdctl node snapshot rollback worker0 pre-upgrade --dry-run
```

### Options

```
      --acknowledge-interrupted-op   override a stranded marker left by an unrelated op and proceed fresh
      --confirm-cluster string       required with --yes; must equal the config cluster name
      --dry-run                      report what would happen without rolling back
  -h, --help                         help for rollback
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

