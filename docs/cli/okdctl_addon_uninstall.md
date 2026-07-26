## okdctl addon uninstall

Uninstall a named addon

### Synopsis

Remove an addon from the cluster.

Uninstall is blocked when any other enabled addon transitively depends on the
target. Disable or uninstall the dependent addon first.

```
okdctl addon uninstall <name> [flags]
```

### Examples

```
  okdctl addon uninstall flux
  okdctl addon uninstall flux --yes --confirm-cluster=prod
```

### Options

```
      --confirm-cluster string   required with --yes; must equal the config cluster name
  -h, --help                     help for uninstall
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

* [okdctl addon](okdctl_addon.md)	 - Manage cluster addons

