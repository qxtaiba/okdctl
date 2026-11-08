# Migrating okdctl.yaml from v2 to v3

This release reads `schemaVersion: v3` only. The loader refuses a v2 file
(or one without a `schemaVersion`) with a pointer to this page instead of
an unknown-field error. The quickest path is `okdctl init`, which writes a
fresh v3 file and the credentials sidecar through the wizard; the tables
below are for carrying an existing v2 file over by hand.

The v3 cluster is always three control-plane nodes plus `workers.count`
workers, installed by the agent-based installer straight onto Proxmox. The
bastion, the ignition HTTP server, the bootstrap VM and Terraform are gone,
and with them every key that configured them.

## Field mapping

| v2 key | v3 key | Note |
| --- | --- | --- |
| `schemaVersion: v2` | `schemaVersion: v3` | |
| `cluster.name`, `cluster.domain` | same | |
| `distribution.type`, `distribution.version` | same | The floor is `4.22.0-okd-scos.9`. |
| `provider.type` | same | |
| `provider.proxmox.host` | `provider.proxmox.endpoint` | A URL with scheme and port, such as `https://192.168.1.100:8006`. |
| `provider.proxmox.node` | `provider.proxmox.nodes` | A list; the VMs spread across it round-robin. |
| `provider.proxmox.control_plane_nodes`, `provider.proxmox.worker_nodes` | `provider.proxmox.nodes` plus `nodes.<name>.host` | The pool replaces the two index lists; a per-node `host` override pins one VM. |
| `provider.proxmox.storage` | same | |
| `provider.proxmox.data_storage` | `<role>.disks[n].storage` | Storage is set per disk; an empty value uses `provider.proxmox.storage`. |
| `provider.proxmox.iso_storage` | same | |
| `provider.proxmox.bridge` | same | Now required. |
| `provider.proxmox.cpu_type` | same | `host` or `x86-64-v3` and newer. |
| `provider.proxmox.ha_enabled` | `provider.proxmox.ha` | |
| `provider.proxmox.additional_networks[].vlan_tag` | `provider.proxmox.vlan` | One tag for the single NIC; `0` means untagged. |
| `topology.vm_id_base` | `provider.proxmox.vmid_base` | Masters take `+10..12`, workers `+100+i`. |
| `topology.control_plane.cpu` | `control_plane.cpu` | |
| `topology.control_plane.memory_mb` | `control_plane.memory_mib` | |
| `topology.control_plane.disk_gb` | `control_plane.disks[0].size_gib` | `disks[0]` is the root disk; the floor is 100 GiB. |
| `topology.workers.count` | `workers.count` | |
| `topology.workers.cpu` | `workers.cpu` | |
| `topology.workers.memory_mb` | `workers.memory_mib` | |
| `topology.workers.disk_gb` | `workers.disks[0].size_gib` | |
| `disks.worker_data_size_gb` | `workers.disks[1].size_gib` | Add `wipe: true` to clear it on first boot. |
| `disks.control_plane_data_size_gb`, `disks.control_plane_mon_size_gb` | `control_plane.disks[1..].size_gib` | One list entry per extra disk. |
| `networking.machine_cidr`, `pod_cidr`, `service_cidr`, `host_prefix`, `gateway`, `dns` | same | `host_prefix` is now required (20..28). |
| `networking.static_ip.start` | `networking.node_ip_start` | In v2 this was the bootstrap address and nodes followed at `+1`; in v3 it is master0's address, masters sit at `+0..2` and workers at `+10+i`. |
| `networking.static_ip.dns` | `networking.dns` | |
| `networking.bastion.vip` | `networking.api_vip` | Required, and `networking.ingress_vip` is new and also required. |
| `networking.ntp_server` | `networking.ntp` | A list; empty keeps the image default. |
| `files.pull_secret`, `files.ssh_public_key` | same | |

Keys new in v3 with no v2 source: `networking.ingress_vip`,
`provider.proxmox.pool`, `nodes` (per-node overrides keyed by
`<cluster>-master<i>` / `<cluster>-worker<i>`), `lifecycle.before_next_node`,
`lifecycle.drain_timeout`, `handoff.enabled` and
`defaults.disable_redhat_catalogs`. The shipped examples under
`configs/examples/` show every one of them with its default.

## Keys with no v3 counterpart

- `networking.bastion.ip`: there is no bastion. The API and ingress VIPs
  are served by the cluster itself.
- `networking.static_ip.interface`, `networking.static_ip.netmask`: the
  agent ISO configures the first NIC by its MAC, and the netmask is read
  from `machine_cidr`.
- `http_server.root`, `http_server.ignition_server_ip`: no ignition files
  are served over HTTP; the install payload is baked into the ISO.
- `deployment.terraform_env`, `deployment.auto_approve`,
  `deployment.bootstrap_timeout`, `deployment.install_timeout`,
  `deployment.bin_dir`: Terraform is gone, and the driver talks to the
  Proxmox API directly.
- `topology.bootstrap.*`, `topology.control_plane.count`: the agent-based
  installer uses master0 as the rendezvous host, so there is no bootstrap
  VM, and the control plane is always three nodes.
- `provider.proxmox.ssh_host_fingerprint`,
  `provider.proxmox.require_pinned_fingerprint`: okdctl no longer opens an
  SSH session to the Proxmox host to place per-node ISOs; the API token is
  the only channel.
- `provider.proxmox.additional_networks`, `provider.proxmox.numa_enabled`:
  one NIC per VM, and no NUMA pinning.
- `addons`: add-ons are not part of the install; `handoff.enabled` applies
  the `handoff/secrets` and `handoff/manifests` directories after it.

## Credentials

The token no longer appears in the YAML in any form, and the loader
refuses a file that assigns `PROXMOX_VE_*` variables. The v2 keys
`provider.proxmox.token_id`, `provider.proxmox.insecure` and
`provider.proxmox.insecure_http` move to `okdctl.env` beside the config,
written at mode 0600:

```sh
PROXMOX_VE_API_TOKEN='user@pve!okdctl=00000000-0000-0000-0000-000000000000'
PROXMOX_VE_INSECURE=true   # only for a self-signed endpoint certificate
```

The same two variables set in the environment take precedence over the
file.

## Starting fresh

Run `okdctl init` to write a v3 file through the wizard, or
`okdctl init --minimal` for the three-node layout without one, then
`okdctl config validate` to see the node table the file produces.
