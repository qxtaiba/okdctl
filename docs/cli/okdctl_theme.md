## okdctl theme

Inspect terminal theme styles

### Options

```
  -h, --help   help for theme
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

* [okdctl](okdctl.md)	 - Provision OKD clusters on Proxmox VE
* [okdctl theme preview](okdctl_theme_preview.md)	 - Preview the TUI color roles

