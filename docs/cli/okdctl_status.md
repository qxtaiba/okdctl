## okdctl status

Print a post-deploy cluster summary

### Synopsis

Print API reachability, node counts by role, cluster operator
health, and addon status for the deployed cluster.

```
okdctl status [flags]
```

### Examples

```
  okdctl status
  okdctl status --output json | jq '.nodes'
  okdctl status --output json | jq '[.nodes[] | select(.ready)] | length'
```

### Options

```
  -h, --help            help for status
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

* [okdctl](okdctl.md)	 - Provision OKD clusters on Proxmox VE

