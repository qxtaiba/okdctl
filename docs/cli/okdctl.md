## okdctl

Provision OKD clusters on Proxmox VE

### Synopsis

okdctl provisions OKD clusters on Proxmox VE from an interactive wizard.
It's for the homelab operator with one or two Proxmox nodes who wants a
real Kubernetes cluster without hand-rolling Terraform, Ignition, and
bootstrap glue.

Release builds check api.github.com for a newer release (at most once
per 24h, cached locally); set OKDCTL_NO_UPDATE_CHECK=1 to disable.

### Options

```
  -c, --config string       configuration file (default "okdctl.yaml")
  -h, --help                help for okdctl
      --log-file string     write log output to this file in addition to stderr (replaces the default okdctl.log sink of deploy/destroy/cleanup)
      --log-format string   log output format: text (TTY default) | json (auto-selected when stderr is piped)
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
  -q, --quiet               suppress info/warn logs (alias for --log-level=error)
  -v, --verbose             enable debug logging (alias for --log-level=debug)
```

### SEE ALSO

* [okdctl addon](okdctl_addon.md)	 - Manage cluster addons
* [okdctl cleanup](okdctl_cleanup.md)	 - Remove OKD cluster artifacts without destroying infrastructure
* [okdctl cluster](okdctl_cluster.md)	 - Manage cluster-wide lifecycle operations
* [okdctl completion](okdctl_completion.md)	 - Generate shell completion script
* [okdctl config](okdctl_config.md)	 - Inspect okdctl configuration
* [okdctl debug-bundle](okdctl_debug-bundle.md)	 - Collect a support bundle for troubleshooting
* [okdctl deploy](okdctl_deploy.md)	 - Deploy an OKD cluster
* [okdctl describe](okdctl_describe.md)	 - Show details for a cluster node or addon
* [okdctl destroy](okdctl_destroy.md)	 - Destroy an OKD cluster
* [okdctl doctor](okdctl_doctor.md)	 - Check that your environment is ready to deploy a cluster
* [okdctl kubeconfig](okdctl_kubeconfig.md)	 - Print or export the cluster kubeconfig
* [okdctl node](okdctl_node.md)	 - Manage cluster node lifecycle
* [okdctl plan](okdctl_plan.md)	 - Preview infrastructure drift without applying changes
* [okdctl releases](okdctl_releases.md)	 - Query available OKD versions
* [okdctl status](okdctl_status.md)	 - Print a post-deploy cluster summary
* [okdctl update-ingress](okdctl_update-ingress.md)	 - Switch ingress DNS from HAProxy to LoadBalancer IPs
* [okdctl version](okdctl_version.md)	 - Print version, git commit, build date

