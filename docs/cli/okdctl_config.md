## okdctl config

Inspect okdctl configuration

### Synopsis

Show and validate the resolved okdctl configuration; start with 'config show' to see the active values.

### Options

```
  -h, --help   help for config
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

* [okdctl](okdctl.md)	 - Provision OKD clusters on Proxmox VE
* [okdctl config show](okdctl_config_show.md)	 - Print the resolved configuration with secrets redacted
* [okdctl config validate](okdctl_config_validate.md)	 - Validate the configuration file and report errors

