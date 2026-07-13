## okdctl node list

List cluster nodes with role, readiness, and sizing drift

### Synopsis

List every cluster node with its role, readiness, terraform count
index, sizing-drift indicator, and any in-flight node op.

The drift indicator compares the config file's per-role cpu/memory to what
was last rendered into terraform.tfvars — it is not a live VM query (okdctl
fetches no per-guest Proxmox sizing anywhere today), so "pending" means a
sizing change is staged in the workspace, not that a specific node's guest
has actually been resized yet. "unknown" means terraform.tfvars has not been
rendered at all.

```
okdctl node list [flags]
```

### Examples

```
  okdctl node list
  okdctl node list --output json
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

* [okdctl node](okdctl_node.md)	 - Manage cluster node lifecycle

