## okdctl plan

Preview infrastructure drift without applying changes

### Synopsis

Run a read-only terraform plan against the current workspace and report
whether the Proxmox infrastructure has drifted from the configuration and
terraform state on disk. okdctl plan never applies changes and never leaves
a usable plan file behind.

Exit code is 0 when the plan is clean, 7 when a create/update/replace/delete
is pending. Run 'okdctl deploy' to reconcile drift.

Pass --output=json for machine-readable output (see docs/cli/json-schema.md).

```
okdctl plan [flags]
```

### Examples

```
  okdctl plan
  okdctl plan --output json | jq '.drift'
```

### Options

```
  -h, --help            help for plan
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

* [okdctl](okdctl.md)	 - Provision OKD clusters on Proxmox VE

