## okdctl debug-bundle

Collect a support bundle for troubleshooting

### Synopsis

Collect a tarball containing redacted configuration, recent log
output, oc adm must-gather results, terraform state summary,
okdctl doctor results, and system metadata.

The output is safe to attach to a support ticket — credentials are
redacted and the raw terraform state file is never included.

Run this after a failed deploy, passing the same --log-file you used
during the deploy so the bundle captures the relevant logs.

```
okdctl debug-bundle [flags]
```

### Options

```
  -h, --help               help for debug-bundle
  -o, --output string      write bundle to this path (default: okdctl-debug-<ts>.tgz)
      --skip-must-gather   skip oc adm must-gather (faster, omits cluster diagnostics)
```

### Options inherited from parent commands

```
  -c, --config string       configuration file (default "okdctl.yaml")
      --log-file string     write log output to this file in addition to stderr
      --log-format string   log output format (text, json) (default "text")
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
  -q, --quiet               suppress info/warn logs (alias for --log-level=error)
  -v, --verbose             enable debug logging (alias for --log-level=debug)
```

### SEE ALSO

* [okdctl](okdctl.md)	 - Deploy production-ready Kubernetes clusters

