## okdctl cleanup

Remove OKD cluster artifacts without destroying infrastructure

### Synopsis

Remove cluster artifacts (work directory, ignition files, HAProxy,
dnsmasq, Apache httpd, Terraform state files) without tearing down
Proxmox infrastructure.

Use this after a manual Terraform destroy, or to reset a failed deployment
to a clean state.

--kind scopes cleanup to a single subsystem instead of the "full" default.

```
okdctl cleanup [flags]
```

### Examples

```
  okdctl cleanup
  okdctl cleanup --yes
  okdctl cleanup --kind work-only
  okdctl cleanup --dry-run
```

### Options

```
      --confirm-cluster string   required with --yes; must equal the config cluster name
      --dry-run                  preview what would be removed without making changes
  -h, --help                     help for cleanup
      --kind string              cleanup scope: full, work-only, web-only, haproxy-only, terraform-only (default "full")
  -y, --yes                      skip confirmation prompt
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

