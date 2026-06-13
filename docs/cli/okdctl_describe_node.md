## okdctl describe node

Show detail for a cluster node

### Synopsis

Show the name, role (master/worker), and readiness condition for a
single cluster node retrieved via oc. Use 'okdctl status' to see all nodes
at once.

```
okdctl describe node <name> [flags]
```

### Examples

```
  okdctl describe node master-0
```

### Options

```
  -h, --help            help for node
  -o, --output string   output format: text|json (default "text")
```

### Options inherited from parent commands

```
  -c, --config string       configuration file (default "okdctl.yaml")
      --log-file string     write log output to this file in addition to stderr
      --log-format string   log output format: text (TTY default) | json (auto-selected when stderr is piped)
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
  -q, --quiet               suppress info/warn logs (alias for --log-level=error)
  -v, --verbose             enable debug logging (alias for --log-level=debug)
```

### SEE ALSO

* [okdctl describe](okdctl_describe.md)	 - Show details for a cluster node or addon

