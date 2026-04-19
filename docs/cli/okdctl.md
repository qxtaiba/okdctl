## okdctl

Deploy production-ready Kubernetes clusters

### Synopsis

Homelab K8s - Deploy production-ready Kubernetes clusters

A delightful CLI tool for deploying OKD/OpenShift clusters
on Proxmox VE infrastructure.

Highlights:
  • Interactive setup wizard with beautiful TUI
  • OKD/OpenShift 4.15-4.21 support
  • Addon-extensible architecture (Flux, secrets, storage, cert-manager)
  • YAML configuration with sensible defaults
  • Automated preflight checks and validation
  • Single binary distribution

```
okdctl [flags]
```

### Options

```
  -c, --config string       configuration file (default "okdctl.yaml")
  -h, --help                help for okdctl
      --log-file string     write log output to this file in addition to stderr
      --log-format string   log output format (text, json) (default "text")
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
  -q, --quiet               suppress info/warn logs (alias for --log-level=error)
  -v, --verbose             enable debug logging (alias for --log-level=debug)
```

### SEE ALSO

* [okdctl addon](okdctl_addon.md)	 - Manage cluster addons
* [okdctl cleanup](okdctl_cleanup.md)	 - Remove OKD cluster artifacts without destroying infrastructure
* [okdctl completion](okdctl_completion.md)	 - Generate shell completion script
* [okdctl config](okdctl_config.md)	 - Inspect okdctl configuration
* [okdctl deploy](okdctl_deploy.md)	 - Deploy a Kubernetes cluster
* [okdctl describe](okdctl_describe.md)	 - Drill into a specific node or addon
* [okdctl destroy](okdctl_destroy.md)	 - Destroy a Kubernetes cluster
* [okdctl doctor](okdctl_doctor.md)	 - Check that your environment is ready to deploy a cluster
* [okdctl kubeconfig](okdctl_kubeconfig.md)	 - Print or export the cluster kubeconfig
* [okdctl releases](okdctl_releases.md)	 - Query available OKD versions
* [okdctl status](okdctl_status.md)	 - Print a post-deploy cluster summary
* [okdctl update-ingress](okdctl_update-ingress.md)	 - Switch ingress DNS from HAProxy to LoadBalancer IPs
* [okdctl version](okdctl_version.md)	 - Print version, git commit, build date

