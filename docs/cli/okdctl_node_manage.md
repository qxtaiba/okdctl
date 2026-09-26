## okdctl node manage

Interactively manage node lifecycle (resize / add / remove)

### Synopsis

Launch the Cluster Lifecycle flow: pick an operation, pick a target from
the live node list, enter parameters, review a real dry-run plan of the
exact blast radius, then execute with the same guards and health gates as
the flag-driven node verbs.

Use --accessible or OKDCTL_ACCESSIBLE=1 for sequential plain-text prompts.

Requires a terminal and an existing configuration; use 'okdctl node
resize/add/remove' for automation.

```
okdctl node manage [flags]
```

### Examples

```
  okdctl node manage
  okdctl node manage --accessible
```

### Options

```
      --accessible   use sequential plain-text prompts instead of the full-screen wizard
  -h, --help         help for manage
```

### Options inherited from parent commands

```
  -c, --config string       configuration file (default "okdctl.yaml")
      --log-file string     also write logs to this file (replaces the default okdctl.log of deploy/destroy/cleanup)
      --log-format string   log output format: text (TTY default) | json (auto-selected when stderr is piped)
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
      --no-color            disable colour and progress output (same as NO_COLOR=1)
      --no-motion           disable TUI animation (same as OKDCTL_NO_MOTION=1; NO_COLOR alone reduces it)
  -q, --quiet               suppress info/warn logs (alias for --log-level=error)
  -v, --verbose             enable debug logging (alias for --log-level=debug)
```

### SEE ALSO

* [okdctl node](okdctl_node.md)	 - Manage cluster node lifecycle

