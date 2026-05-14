## okdctl deploy

Deploy a Kubernetes cluster

### Synopsis

Deploy an OKD/OpenShift cluster through an interactive wizard.

```
okdctl deploy [flags]
```

### Examples

```
  okdctl deploy
  okdctl deploy --config my-cluster.yaml
  okdctl deploy --yes --output-file my-cluster.yaml
  okdctl deploy --dry-run
```

### Options

```
      --dry-run                 preview terraform plan and step listing without deploying
  -h, --help                    help for deploy
      --metrics-addr string     address for Prometheus metrics endpoint; bare ":9090" binds 127.0.0.1; disabled when empty
      --metrics-allow-network   allow metrics endpoint to bind on a wildcard address (0.0.0.0 or [::])
      --minimal                 use minimal defaults (single-node cluster)
      --output-file string      output file for configuration (default "okdctl.yaml")
  -y, --yes                     skip prompts, use defaults
```

### Options inherited from parent commands

```
  -c, --config string       configuration file (default "okdctl.yaml")
      --log-file string     write log output to this file in addition to stderr
      --log-format string   log output format: text (TTY default) | json (auto-selected when stderr is piped)
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
  -q, --quiet               suppress info/warn logs (alias for --log-level=error)
  -v, --verbose             enable debug logging (alias for --log-level=debug)
```

### SEE ALSO

* [okdctl](okdctl.md)	 - Deploy production-ready Kubernetes clusters

