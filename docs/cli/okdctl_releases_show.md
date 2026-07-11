## okdctl releases show

Show release info for a single OKD version

### Synopsis

Print metadata for a single OKD release identified by its version string
("4.21.3") or GitHub tag. The version list is resolved from the disk cache;
use --channel=all with 'releases list' to discover pre-release tags.

```
okdctl releases show <version> [flags]
```

### Examples

```
  okdctl releases show 4.21.3
  okdctl releases show 4.21.3 --output json
```

### Options

```
  -h, --help            help for show
  -o, --output string   output format: text|json (default "text")
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

* [okdctl releases](okdctl_releases.md)	 - Query available OKD versions

