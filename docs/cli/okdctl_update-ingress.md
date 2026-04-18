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

### Options

```
  -h, --help             help for update-ingress
      --remove-haproxy   remove haproxy from bastion after dns switch (default true)
  -y, --yes              skip confirmation prompts
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

