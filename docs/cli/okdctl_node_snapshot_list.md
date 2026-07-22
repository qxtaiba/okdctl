## okdctl node snapshot list

List a node's Proxmox snapshots

### Synopsis

List target's Proxmox snapshots. Read-only; no confirmation gate.

```
okdctl node snapshot list <node> [flags]
```

### Examples

```
  okdctl node snapshot list worker0
  okdctl node snapshot list worker0 --output json
```

### Options

```
  -h, --help            help for list
  -o, --output string   output format: text|json (default "text")
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

