## okdctl deploy

Deploy a Kubernetes cluster

### Synopsis

Deploy an OKD/OpenShift cluster through an interactive wizard.

```
okdctl deploy [flags]
```

### Options

```
      --airgap                activate air-gap mode; use mirror resolver for all fetches
      --dry-run               preview terraform plan and step listing without deploying
  -h, --help                  help for deploy
      --metrics-addr string   address for Prometheus metrics endpoint (e.g. :9090); disabled when empty
      --minimal               use minimal defaults (single-node cluster)
  -o, --output string         output file for configuration (default "okdctl.yaml")
  -y, --yes                   skip prompts, use defaults
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

