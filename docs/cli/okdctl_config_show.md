## okdctl config show

Print the resolved configuration with secrets redacted

### Synopsis

Print the fully-resolved okdctl configuration with all secret fields
replaced by "***". Text mode emits YAML; pass --output=json to get a JSON
object suitable for piping to jq.

```
okdctl config show [flags]
```

### Examples

```
  okdctl config show
  okdctl config show --output json | jq '.provider'
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

* [okdctl config](okdctl_config.md)	 - Inspect okdctl configuration

