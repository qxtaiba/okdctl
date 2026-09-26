## okdctl theme preview

Preview the TUI color roles

### Synopsis

Render okdctl's semantic TUI styles in dark, light, high-contrast, and
terminal-inherit modes. The preview is read-only and uses the current terminal
color profile; NO_COLOR and --no-color suppress ANSI styling.

Terminal-inherit uses the active resolved theme polarity. Interactive screens
can update that polarity from the terminal's background-color response; when
no response is available, okdctl keeps its dark-background default.

```
okdctl theme preview [flags]
```

### Examples

```
  okdctl theme preview
  okdctl theme preview --mode light
  NO_COLOR=1 okdctl theme preview --mode inherit
```

### Options

```
  -h, --help          help for preview
      --mode string   preview one mode: all|dark|light|high-contrast|inherit (default "all")
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

* [okdctl theme](okdctl_theme.md)	 - Inspect terminal theme styles

