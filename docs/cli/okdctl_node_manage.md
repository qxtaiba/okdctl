## okdctl node manage

Interactively manage node lifecycle (resize / add / remove)

### Synopsis

Launch the Cluster Lifecycle wizard: pick an operation, pick a target from
the live node list, enter parameters, review a real dry-run plan of the
exact blast radius, then execute with the same guards and health gates as
the flag-driven node verbs.

Requires a terminal and an existing configuration; use 'okdctl node
resize/add/remove' for automation.

```
okdctl node manage [flags]
```

### Examples

```
  okdctl node manage
```

### Options

```
  -h, --help   help for manage
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

