## okdctl deploy

Deploy an OKD cluster

### Synopsis

Deploy an OKD cluster through an interactive wizard.

Use --yes with --confirm-cluster to skip the wizard and deploy
non-interactively from an existing configuration file (and its okdctl.env
credential sidecar) — no TTY required, so a failed deploy can be resumed
over SSH or from CI. --confirm-cluster must equal the configured cluster
name, the same guard every other scripted lifecycle command carries.
Use --write-config to write the configuration file non-interactively
without deploying.

```
okdctl deploy [flags]
```

### Examples

```
  okdctl deploy
  okdctl deploy --config my-cluster.yaml
  okdctl deploy --yes --confirm-cluster=prod         # scripted deploy from okdctl.yaml, no wizard
  okdctl deploy --write-config --output-file my-cluster.yaml  # writes config only; does not deploy
  okdctl deploy --dry-run
  okdctl deploy --keep-redhat-catalogs
```

### Options

```
      --acknowledge-interrupted-op   deploy despite an in-flight node op marker (deploy would otherwise refuse: reconciling mid-op destroys the in-flight node)
      --confirm-cluster string       required with --yes; must equal the config cluster name
      --dry-run                      preview terraform plan and step listing without deploying
      --fresh                        wipe the work directory even when live cluster state is detected (credentials will be lost)
  -h, --help                         help for deploy
      --keep-redhat-catalogs         keep the Red Hat OperatorHub catalogsources and the InsightsDisabled alert
      --minimal                      use minimal defaults (single-node cluster)
      --output-file string           config file to write wizard output to (reused if present; overrides --config) (default "okdctl.yaml")
      --write-config                 write configuration non-interactively; does not deploy
  -y, --yes                          skip the wizard and deploy from the existing configuration file (requires --confirm-cluster)
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

