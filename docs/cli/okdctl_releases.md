## okdctl releases

Query available OKD versions

### Synopsis

List and inspect OKD releases resolved from the GitHub releases feed.

### Options

```
  -h, --help   help for releases
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

* [okdctl](okdctl.md)	 - Deploy production-ready Kubernetes clusters
* [okdctl releases list](okdctl_releases_list.md)	 - List available OKD versions
* [okdctl releases show](okdctl_releases_show.md)	 - Show release info for a single OKD version

