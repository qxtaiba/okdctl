## okdctl deploy

Deploy a Kubernetes cluster

### Synopsis

Deploy an OKD/OpenShift cluster through an interactive wizard.

```
okdctl deploy [flags]
```

### Options

```
  -h, --help              help for deploy
      --minimal           use minimal defaults (single-node cluster)
      --non-interactive   use all defaults without prompts
  -o, --output string     output file for configuration (default "okdctl.yaml")
  -y, --yes               skip prompts, use defaults (alias for --non-interactive)
```

### Options inherited from parent commands

```
  -c, --config string       configuration file (default "okdctl.yaml")
      --log-file string     write log output to this file in addition to stdout
      --log-format string   log output format (text, json) (default "text")
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
```

### SEE ALSO

* [okdctl](okdctl.md)	 - Deploy production-ready Kubernetes clusters

