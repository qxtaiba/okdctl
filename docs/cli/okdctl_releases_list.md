## okdctl releases list

List available OKD versions

```
okdctl releases list [flags]
```

### Examples

```
  okdctl releases list
  okdctl releases list --channel all
  okdctl releases list --format json
```

### Options

```
      --channel string   filter versions: stable|all (default "stable")
  -F, --format string    output format: text|json (default "text")
  -h, --help             help for list
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

* [okdctl releases](okdctl_releases.md)	 - Query available OKD versions

