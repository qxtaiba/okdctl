## okdctl destroy

Destroy a Kubernetes cluster

### Synopsis

Destroy a Kubernetes cluster and all associated infrastructure.
This operation is idempotent and safe to re-run if a previous destroy was interrupted.

Use --dry-run to preview the terraform destroy plan without modifying infra.

Master nodes ship with prevent_destroy = true in the Terraform module to
guard against accidental etcd-quorum loss. To run a full or targeted
destroy, place an override.tf in
infrastructure/terraform/modules/proxmox-okd/ disabling prevent_destroy on
the master resource:

  resource "proxmox_virtual_environment_vm" "master" {
    lifecycle {
      prevent_destroy = false
    }
  }

Remove the override.tf after destroy completes. Alternatively, pass
--skip-terraform to bypass Terraform entirely and remove VMs by hand.

```
okdctl destroy [flags]
```

### Examples

```
  okdctl destroy                              # interactive prompt
  okdctl destroy --yes --confirm-cluster=prod # scripted destroy
  okdctl destroy --dry-run
```

### Options

```
      --confirm-cluster string   required with --yes; must equal cfg.Cluster.Name (typo guard for scripted destroys)
      --dry-run                  preview terraform destroy plan without running destroy
  -h, --help                     help for destroy
      --keep-isos                do not remove the FCOS ISO from the Proxmox host
      --skip-cleanup             skip host file cleanup — leaves haproxy/dnsmasq config in place (no-op with --dry-run)
      --skip-firewall            skip firewall rule cleanup (no-op with --dry-run)
      --skip-terraform           skip terraform destroy — intended for resuming after a successful terraform-destroy phase (no-op with --dry-run)
      --target stringArray       limit terraform destroy to this resource address (repeatable); must match the okd_cluster VM allowlist
  -y, --yes                      skip confirmation prompt
```

### Options inherited from parent commands

```
  -c, --config string       configuration file (default "okdctl.yaml")
      --log-file string     write log output to this file in addition to stderr
      --log-format string   log output format (text, json); defaults to json when stderr is not a TTY (pass --log-format=text to keep text output in pipes) (default "text")
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
  -q, --quiet               suppress info/warn logs (alias for --log-level=error)
  -v, --verbose             enable debug logging (alias for --log-level=debug)
```

### SEE ALSO

* [okdctl](okdctl.md)	 - Deploy production-ready Kubernetes clusters

