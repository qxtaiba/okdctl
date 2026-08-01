## okdctl destroy

Destroy an OKD cluster

### Synopsis

Destroy an OKD cluster and all associated infrastructure.
This operation is idempotent and safe to re-run if a previous destroy was interrupted.

Use --dry-run to preview the terraform destroy plan without modifying infra.
dry-run previews the terraform-destroy plan; the --skip-* flags resume a
partial terraform-destroy — the two address different failure points and
cannot be combined (see the --dry-run incompatibility check).

A scoped destroy (--target or --only) only tears down the named Terraform
resources; host cleanup (haproxy/dnsmasq config, kubeconfig, terraform state
files), firewall rules, and Proxmox ISO removal are skipped automatically for
a scoped run — that bastion-wide teardown runs only on an unscoped destroy,
so it never touches a still-running control plane.

Master nodes ship with prevent_destroy = true in the Terraform module to
guard against accidental etcd-quorum loss. A fully-confirmed destroy
handles this automatically: after the confirmation gate passes, okdctl
writes a transient prevent_destroy_override.tf into
infrastructure/terraform/modules/proxmox-okd/ and removes it when the
destroy finishes (success or failure). Every non-destroy command refuses
to plan or apply while that file exists, so a stale copy from a crashed
run must be deleted by hand — the refusal names the path. If you must
override manually, an override file only merges within its own module, so
it belongs in the modules/proxmox-okd/ directory (never under
environments/). Alternatively, pass --skip-terraform to bypass Terraform
entirely and remove VMs by hand.

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
      --confirm-cluster string   required with --yes; must equal the config cluster name
      --dry-run                  preview terraform destroy plan without running destroy
  -h, --help                     help for destroy
      --keep-isos                do not remove the FCOS ISO from the Proxmox host (always true for a scoped --target/--only destroy)
      --only string              scope destroy to a node group: vms, workers, masters, bootstrap (expands into --target; mutually exclusive with --target; scopes cleanup/firewall/iso removal off automatically)
      --skip-cleanup             skip host file cleanup — leaves haproxy/dnsmasq config in place (no-op with --dry-run; always true for a scoped --target/--only destroy)
      --skip-firewall            skip firewall rule cleanup (no-op with --dry-run; always true for a scoped --target/--only destroy)
      --skip-terraform           skip terraform destroy — intended for resuming after a successful terraform-destroy phase (no-op with --dry-run)
      --target stringArray       limit terraform destroy to this resource address (repeatable); must match the okd_cluster VM allowlist; scopes cleanup/firewall/iso removal off automatically
  -y, --yes                      skip confirmation prompt
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

