## okdctl addon verify

Verify health of all enabled addons

### Synopsis

Run each enabled addon's Verify() probe against the live cluster and
report pass/fail for every addon. The output lists each addon name alongside
OK or a FAIL reason. Exit code is non-zero if any probe fails or if the
configuration cannot be loaded.

See also: addon list

```
okdctl addon verify [flags]
```

### Examples

```
  okdctl addon verify
```

### Options

```
  -h, --help            help for verify
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

* [okdctl addon](okdctl_addon.md)	 - Manage cluster addons

