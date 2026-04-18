## okdctl addon uninstall

Uninstall a named addon

### Synopsis

Remove an addon from the cluster.

Uninstall is blocked when any other enabled addon transitively depends on the
target. Disable or uninstall the dependent addon first.

```
okdctl addon uninstall <name> [flags]
```

### Options

```
  -h, --help   help for uninstall
```

### Options inherited from parent commands

```
  -c, --config string       configuration file (default "okdctl.yaml")
      --log-file string     write log output to this file in addition to stdout
      --log-format string   log output format (text, json) (default "text")
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
```

### SEE ALSO

* [okdctl addon](okdctl_addon.md)	 - Manage cluster addons

