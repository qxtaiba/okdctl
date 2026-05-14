## okdctl describe

Drill into a specific node or addon

### Synopsis

Inspect a cluster node or registered addon in detail; start with 'describe node <name>'.

### Options

```
  -h, --help   help for describe
```

### Options inherited from parent commands

```
  -c, --config string       configuration file (default "okdctl.yaml")
      --log-file string     write log output to this file in addition to stderr
      --log-format string   log output format (text, json); defaults to json when stderr is not a TTY (pass --log-format=text to keep text output in pipes) (default "text")
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
  -q, --quiet               suppress info/warn logs (alias for --log-level=error)
  -v, --verbose             enable debug logging (alias for --log-level=debug)
```

### SEE ALSO

* [okdctl](okdctl.md)	 - Deploy production-ready Kubernetes clusters
* [okdctl describe addon](okdctl_describe_addon.md)	 - Show detail for a registered addon
* [okdctl describe node](okdctl_describe_node.md)	 - Show detail for a cluster node

