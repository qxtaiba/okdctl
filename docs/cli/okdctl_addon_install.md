## okdctl addon install

Install one addon (or all enabled addons with --all)

### Synopsis

Install an addon onto the live cluster.

install <name>  installs the named addon and its transitive dependencies.
                If any addon in the dependency closure fails, all addons
                installed in this invocation are uninstalled in reverse order
                before the error is returned (all-or-nothing rollback).

install --all   installs every addon enabled in the configuration file in
                dependency order. If an individual addon fails it is rolled back
                in isolation; unrelated addons continue installing
                (per-addon continuation).

```
okdctl addon install [name] [flags]
```

### Examples

```
  okdctl addon install flux
  okdctl addon install --all
```

### Options

```
      --all    install all enabled addons (per-addon continuation on failure)
  -h, --help   help for install
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

* [okdctl addon](okdctl_addon.md)	 - Manage cluster addons

