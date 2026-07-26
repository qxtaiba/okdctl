## okdctl deploy

Deploy a Kubernetes cluster

### Synopsis

Deploy an OKD/OpenShift cluster through an interactive wizard.

Use --yes to skip the wizard and deploy non-interactively from an existing
configuration file. Use --write-config to write the configuration file
non-interactively without deploying.

Note: before v0.2.0, --yes meant what --write-config means now.

```
okdctl deploy [flags]
```

### Examples

```
  okdctl deploy
  okdctl deploy --config my-cluster.yaml
  okdctl deploy --yes                                # deploys from okdctl.yaml, no wizard
  okdctl deploy --write-config --output-file my-cluster.yaml  # writes config only; does not deploy
  okdctl deploy --dry-run
  okdctl deploy --keep-redhat-catalogs
```

### Options

```
      --dry-run                preview terraform plan and step listing without deploying
      --fresh                  wipe the work directory even when live cluster state is detected (credentials will be lost)
  -h, --help                   help for deploy
      --keep-redhat-catalogs   keep the redhat-operators, certified-operators, and redhat-marketplace OperatorHub catalogsources and the InsightsDisabled alert enabled (both require a Red Hat subscription OKD clusters don't have)
      --minimal                use minimal defaults (single-node cluster)
      --output-file string     config file to write wizard output to; reuses and reads back an existing file at this path, otherwise creates one; overrides --config when both are set (default "okdctl.yaml")
      --write-config           write configuration non-interactively; does not deploy
  -y, --yes                    skip the wizard and deploy from the existing configuration file
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

