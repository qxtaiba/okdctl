## okdctl cleanup

Remove OKD cluster artifacts without destroying infrastructure

### Synopsis

Remove cluster artifacts (work directory, ignition files, HAProxy,
dnsmasq, Apache httpd, Terraform state files) without tearing down
Proxmox infrastructure.

Use this after a manual Terraform destroy, or to reset a failed deployment
to a clean state.

```
okdctl cleanup [flags]
```

### Options

```
  -h, --help   help for cleanup
  -y, --yes    skip confirmation prompt
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

