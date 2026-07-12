# The workspace contract

Run `deploy` from any directory — an empty one works. One working
directory is one cluster: okdctl writes everything it needs next to where
you run it.

## What a deploy writes

First run materializes the Terraform sources embedded in the binary into
`infrastructure/terraform/` (write-once — files already present, e.g. from
a source checkout or hand-edited HCL, are never overwritten), launches the
wizard, and writes `okdctl.yaml` plus an `okdctl.env` file next to the
config (named after the config file) holding Proxmox credentials. The
deploy itself adds an `okd-install/` work directory and Terraform state
under `infrastructure/terraform/environments/`.

`deploy`, `destroy`, and `cleanup` append their full log to `okdctl.log`
next to the config (credentials redacted, one `run_id` per invocation), so
a failed deploy stays diagnosable after the terminal scrollback is gone.
`okdctl debug-bundle` picks the log up automatically; `--log-file`
redirects it.

## Re-running and multiple clusters

Later runs reuse the existing config. Commands that operate on an existing
cluster (`status`, `destroy`, `kubeconfig`, `debug-bundle`) must run from
the same directory the deploy used. To manage multiple clusters, use one
directory per cluster (`--config other.yaml` selects an alternate config
file within one directory).

If the work directory holds a completed cluster, run `okdctl destroy`
before deploying again; `--fresh` force-wipes the work directory without
destroying infrastructure (credentials will be lost).

`okdctl destroy` also removes the dnsmasq drop-in, the HAProxy config
block, and the firewall rules okdctl added, along with the
Terraform-provisioned VMs and the `haproxy`, `dnsmasq`, and `httpd`
packages it installed.

## Proxmox credentials

Proxmox credentials live in the `okdctl.env` file next to the config,
never in the YAML: `PROXMOX_VE_ENDPOINT`, `PROXMOX_VE_USERNAME`,
`PROXMOX_VE_PASSWORD` (or `PROXMOX_VE_API_TOKEN`).

## Example configs

Reference configs live in [configs/examples/](../../configs/examples/),
each documented with a header comment: `minimal.yaml` (single
control-plane node, single-node cluster), `production.yaml` (3
control-plane, 5 worker layout), `media-server.yaml` (homelab layout with
storage-heavy workers).
