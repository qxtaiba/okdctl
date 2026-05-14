## okdctl config validate

Validate the configuration file and report errors

```
okdctl config validate [flags]
```

### Examples

```
  okdctl config validate
```

### Options

```
  -h, --help   help for validate
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

* [okdctl config](okdctl_config.md)	 - Inspect okdctl configuration

