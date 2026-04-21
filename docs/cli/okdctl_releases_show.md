## okdctl releases show

Show release info for a single OKD version

```
okdctl releases show <version> [flags]
```

### Examples

```
  okdctl releases show 4.21.3
  okdctl releases show 4.21.3 --format json
```

### Options

```
  -F, --format string   output format: text|json (default "text")
  -h, --help            help for show
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

