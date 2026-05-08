## okdctl doctor

Check that your environment is ready to deploy a cluster

### Synopsis

Run preflight checks on the local environment before a deploy.

Each check prints a title line with a status icon and a result line
with a bracketed label:

  ✓ [ok]   : the check passed, no action needed
  ⚠ [warn] : something is suboptimal or missing but can be handled
             during deploy (e.g., 'oc' will be auto-downloaded into
             /usr/local/bin)
  ✗ [fail] : this must be fixed before 'okdctl deploy' will
             succeed

Exit code is 0 if there are no [fail] results ([warn] is tolerated),
2 (configuration error) otherwise. Designed to be rerun until clean.

Pass --output=json for machine-readable output (see docs/cli/json-schema.md).

See docs/doctor-checks.md for per-check fail messages and fix guidance.

```
okdctl doctor [flags]
```

### Examples

```
  okdctl doctor
  okdctl doctor --output json | jq '.failed'
```

### Options

```
  -h, --help            help for doctor
  -o, --output string   output format: text|json (default "text")
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

