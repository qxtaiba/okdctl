## okdctl releases list

List available OKD versions

### Synopsis

List OKD versions resolved from the GitHub releases feed.

By default only stable releases are shown; pass --channel=all to include every
non-draft release. Results are served from a 1-hour on-disk cache
(~/.okdctl/cache/okd-versions.json) to avoid repeated network round-trips.

```
okdctl releases list [flags]
```

### Examples

```
  okdctl releases list
  okdctl releases list --channel all
  okdctl releases list --output json
```

### Options

```
      --channel string   filter versions: stable|all (default "stable")
  -h, --help             help for list
  -o, --output string    output format: text|json (default "text")
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

