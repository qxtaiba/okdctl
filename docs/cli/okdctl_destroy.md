## okdctl destroy

Destroy a Kubernetes cluster

### Synopsis

Destroy a Kubernetes cluster and all associated infrastructure.
This operation is idempotent and safe to re-run if a previous destroy was interrupted.

```
okdctl destroy [flags]
```

### Options

```
  -y, --force       skip confirmation prompt
  -h, --help        help for destroy
      --keep-isos   do not remove the FCOS ISO from the Proxmox host
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

