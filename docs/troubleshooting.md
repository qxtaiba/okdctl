# Troubleshooting

Run `okdctl doctor` first — it catches most common failures, and its
output belongs in bug reports.

- **Bootstrap VM never comes up.** Almost always networking: the ignition
  URL must be reachable from the node network
  (`https://<ignition_server_ip>/ignition/<role>.ign`, port 443). `okdctl
  doctor` probes this.
- **`dnsmasq` fails on port 53.** `systemd-resolved` already has it. Set
  `DNSStubListener=no` in `/etc/systemd/resolved.conf`, restart
  `systemd-resolved`, then retry `okdctl deploy`.
- **`oc` not found mid-setup.** okdctl installs it into `/usr/local/bin`,
  which isn't on `$PATH` in the current shell until you re-source your rc
  file.
- **Terraform destroy hangs.** The Proxmox API drops long-running destroy
  requests under load. Re-run `okdctl destroy` — state is preserved.
- **CSR approval fails on clock skew.** Nodes whose clock differs from the
  bastion's get their certs refused. Run `ntpdate` on both and retry.

Preflight check reference: [docs/doctor-checks.md](doctor-checks.md).
