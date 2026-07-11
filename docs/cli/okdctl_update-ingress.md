## okdctl update-ingress

Switch ingress DNS from HAProxy to LoadBalancer IPs

### Synopsis

Detect IngressController strategies and LoadBalancer IPs, then update
DNS records to point *.apps at the real LoadBalancer IP instead of
the bastion HAProxy.

If any IngressControllers use HostNetwork (common on bare-metal OKD)
and MetalLB is available, you will be prompted to convert them to
LoadBalancerService. This requires deleting and recreating the
IngressController, which causes a brief outage (~30s) for routes on
affected controllers.

Run this after deploying a LoadBalancer provider (e.g., MetalLB).

```
okdctl update-ingress [flags]
```

### Examples

```
  okdctl update-ingress
  okdctl update-ingress --yes --keep-haproxy
  okdctl update-ingress --dry-run
```

### Options

```
      --confirm-cluster string   required with --yes; must equal cfg.Cluster.Name (typo guard for scripted update-ingress runs)
      --dry-run                  preview update-ingress mutations without touching the cluster
  -h, --help                     help for update-ingress
      --keep-haproxy             keep haproxy running on the bastion after dns switch
  -y, --yes                      skip confirmation prompts
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

* [okdctl](okdctl.md)	 - Deploy production-ready Kubernetes clusters

