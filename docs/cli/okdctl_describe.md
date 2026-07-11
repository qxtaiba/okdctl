## okdctl describe

Show details for a cluster node or addon

### Synopsis

Show detailed information for a specific cluster node or registered addon.

### Options

```
  -h, --help   help for describe
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

* [okdctl](okdctl.md)	 - Provision OKD clusters on Proxmox VE
* [okdctl describe addon](okdctl_describe_addon.md)	 - Show detail for a registered addon
* [okdctl describe node](okdctl_describe_node.md)	 - Show detail for a cluster node

