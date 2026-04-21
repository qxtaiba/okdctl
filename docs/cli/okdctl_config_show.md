## okdctl config show

Print the resolved configuration with secrets redacted

```
okdctl config show [flags]
```

### Examples

```
  okdctl config show
```

### Options

```
  -h, --help   help for show
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

* [okdctl config](okdctl_config.md)	 - Inspect okdctl configuration

