# Security considerations

To report a vulnerability, see [SECURITY.md](../SECURITY.md).

## Ignition pull-secret exposure window

During bootstrap (roughly 15-30 minutes), Apache on the bastion serves
`bootstrap.ign`, `master.ign`, and `worker.ign` over HTTPS on port 443.
These files embed the OKD pull-secret JSON in plain text.

okdctl binds Apache to `http_server.ignition_server_ip` (the bridge IP FCOS
nodes reference in their kargs ignition URL), not `0.0.0.0`, so hosts off
the machine network can't reach it. Each node ISO gets the server's CA
embedded via `coreos-installer iso customize --ignition-ca`, so nodes
verify the server before requesting files. The residual risk: TLS
authenticates the server, not the client, so any host that can reach the
bastion bridge IP on port 443 during bootstrap can retrieve the ignition
files and harvest the pull secret.

Mitigations:

- Isolate the bastion bridge network from untrusted hosts (VLAN, private
  bridge, or Proxmox SDN zone) before running `okdctl deploy`.
- Run `okdctl cleanup` after deploy completes — it removes the ignition
  files from the web root.

## SSH/SCP host-key trust on first run (TOFU)

The first `okdctl deploy` scps CoreOS ISOs to the Proxmox host with
`-o StrictHostKeyChecking=accept-new`, trusting and pinning the Proxmox
host key without prior verification. Every later SSH/SCP call (Proxmox
shell commands, ISO removal) reuses that cached key.

A machine-in-the-middle on the bastion-to-Proxmox path during that first
SCP call can substitute an attacker key, which then stays trusted for the
life of the cluster.

Set `provider.proxmox.ssh_host_fingerprint` in `okdctl.yaml`
(`SHA256:<base64>`, from `ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub`
or the Proxmox UI) for deterministic verification — every later SSH/SCP
call then refuses on mismatch. Set
`provider.proxmox.require_pinned_fingerprint: true` to fail closed when
the pin is absent.

Other mitigations:

- Deploy from a bastion with a trusted L2 path to the Proxmox host (no
  NAT or L3 hop an attacker can sit on).
- Before the first deploy, SSH to the Proxmox host manually and verify its
  fingerprint out-of-band (Proxmox UI → Node → Shell → `ssh-keygen -lf
  /etc/ssh/ssh_host_ed25519_key.pub`). Once it's in `~/.ssh/known_hosts`,
  `accept-new` won't override it.
