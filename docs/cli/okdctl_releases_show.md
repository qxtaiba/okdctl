## okdctl releases show

Show release info for a single OKD version

### Synopsis

Print metadata for a single OKD release identified by its full GitHub tag
(e.g. "4.21.3-okd-scos.0"). The version list is resolved from the disk cache;
use --channel=all with 'releases list' to discover pre-release tags.

```
okdctl releases show <version> [flags]
```

### Examples

```
  okdctl releases show 4.21.3-okd-scos.0
  okdctl releases show 4.21.3-okd-scos.0 --output json
```

### Options

```
  -h, --help            help for show
  -o, --output string   output format: text|json (default "text")
```

### Options inherited from parent commands

```
  -c, --config string       configuration file (default "okdctl.yaml")
      --log-file string     also write logs to this file (replaces the default okdctl.log of deploy/destroy/cleanup)
      --log-format string   log output format: text (TTY default) | json (auto-selected when stderr is piped)
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
      --no-color            disable colour and progress output (same as NO_COLOR=1)
      --no-motion           disable TUI animation (same as OKDCTL_NO_MOTION=1; NO_COLOR alone reduces it)
  -q, --quiet               suppress info/warn logs (alias for --log-level=error)
  -v, --verbose             enable debug logging (alias for --log-level=debug)
```

### SEE ALSO

* [okdctl releases](okdctl_releases.md)	 - Query available OKD versions

