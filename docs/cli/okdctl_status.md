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
  okdctl status --format json | jq .ready_nodes
```

### Options

```
  -F, --format string   output format: text|json (default "text")
  -h, --help            help for status
```

### Options inherited from parent commands

```
  -c, --config string       configuration file (default "okdctl.yaml")
      --log-file string     write log output to this file in addition to stderr
      --log-format string   log output format (text, json) (default "text")
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
  -q, --quiet               suppress info/warn logs (alias for --log-level=error)
  -v, --verbose             enable debug logging (alias for --log-level=debug)
```

### SEE ALSO

* [okdctl](okdctl.md)	 - Deploy production-ready Kubernetes clusters

