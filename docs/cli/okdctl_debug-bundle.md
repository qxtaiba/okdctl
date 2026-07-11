## okdctl debug-bundle

Collect a support bundle for troubleshooting

### Synopsis

Collect a tarball containing redacted configuration, recent log
output, oc adm must-gather results, terraform state summary,
okdctl doctor results, and system metadata.

The output is safe to attach to a support ticket — credentials are
redacted and the raw terraform state file is never included.

Run this after a failed deploy. deploy, destroy, and cleanup append
their full log to okdctl.log in the working directory by default and
the bundle picks it up automatically; pass the same --log-file the
failing run used only if it overrode that default.

Pass --quiet to suppress progress logs to stderr when only the bundle
file is needed (e.g. in scripts or CI).

```
okdctl debug-bundle [flags]
```

### Examples

```
  okdctl debug-bundle
  okdctl debug-bundle --output-file my-cluster.tgz
  okdctl debug-bundle --skip-must-gather
```

### Options

```
  -h, --help                 help for debug-bundle
      --output-file string   write bundle to this path; empty auto-generates okdctl-debug-<ts>.tgz in the cwd; overwrites an existing file at that path
      --skip-must-gather     skip oc adm must-gather (faster, omits cluster diagnostics)
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

