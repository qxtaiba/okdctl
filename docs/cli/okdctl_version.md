## okdctl version

Print version, git commit, build date

### Synopsis

Print the okdctl build identity: version number, git commit SHA,
build date, Go toolchain version, and OS/arch platform.

Pass --output=json for machine-readable output suitable for CI version
pinning or scripted comparisons (see docs/cli/json-schema.md).

```
okdctl version [flags]
```

### Examples

```
  okdctl version
  okdctl version --output json | jq .version
```

### Options

```
  -h, --help            help for version
  -o, --output string   output format: text|json (default "text")
```

### Options inherited from parent commands

```
  -c, --config string       configuration file (default "okdctl.yaml")
      --log-file string     also write logs to this file (replaces the default okdctl.log of deploy/destroy/cleanup)
      --log-format string   log output format: text (TTY default) | json (auto-selected when stderr is piped)
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
      --no-color            disable colour and progress output (same as NO_COLOR=1)
  -q, --quiet               suppress info/warn logs (alias for --log-level=error)
  -v, --verbose             enable debug logging (alias for --log-level=debug)
```

### SEE ALSO

* [okdctl](okdctl.md)	 - Provision OKD clusters on Proxmox VE

