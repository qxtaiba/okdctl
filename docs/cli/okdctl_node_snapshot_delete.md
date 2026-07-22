## okdctl node snapshot delete

Delete a node's Proxmox snapshot

### Synopsis

Remove name from target. Does not touch VM power state or cordon status.

```
okdctl node snapshot delete <node> <name> [flags]
```

### Examples

```
  okdctl node snapshot delete worker0 pre-upgrade --yes --confirm-cluster grappleberry
```

### Options

```
      --confirm-cluster string   required with --yes; must equal the config cluster name
  -h, --help                     help for delete
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

* [okdctl node snapshot](okdctl_node_snapshot.md)	 - Manual, single-node Proxmox VM snapshots

