## okdctl config

Inspect okdctl configuration

### Options

```
  -h, --help   help for config
```

### Options inherited from parent commands

```
  -c, --config string       configuration file (default "okdctl.yaml")
      --log-file string     write log output to this file in addition to stderr
      --log-format string   log output format (text, json); defaults to json when stderr is not a TTY (pass --log-format=text to keep text output in pipes) (default "text")
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
  -q, --quiet               suppress info/warn logs (alias for --log-level=error)
  -v, --verbose             enable debug logging (alias for --log-level=debug)
```

### SEE ALSO

* [okdctl](okdctl.md)	 - Deploy production-ready Kubernetes clusters
* [okdctl config show](okdctl_config_show.md)	 - Print the resolved configuration with secrets redacted
* [okdctl config validate](okdctl_config_validate.md)	 - Validate the configuration file and report errors

