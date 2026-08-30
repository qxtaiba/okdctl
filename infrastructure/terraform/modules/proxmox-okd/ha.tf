# Opt-in (ha_enabled): HA-manages master VMs with anti-affinity spreading
# across proxmox nodes. Supersedes main.tf's startup{} ordering on failover;
# CRM behavior on an out-of-band shutdown is unverified on PVE9 (hence the
# stop/start warnings when enabled).

data "proxmox_version" "current" {
  count = var.ha_enabled ? 1 : 0

  lifecycle {
    postcondition {
      condition     = tonumber(split(".", self.release)[0]) >= 9
      error_message = "proxmox ha anti-affinity (harule resource-affinity) requires pve 9+; detected release ${self.release}"
    }
  }
}

# for_each dereferences all masters (count-based, not for_each), so a
# master-scoped destroy fans across all masters or orphans the vm:<id> CRM
# entry; prevent_destroy (main.tf) blocks that until the override is used.
resource "proxmox_haresource" "master" {
  for_each = var.ha_enabled ? { for idx, name in local.masters : name => proxmox_virtual_environment_vm.master[idx].vm_id } : {}

  depends_on = [data.proxmox_version.current]

  resource_id = "vm:${each.value}"
  state       = "started"

  # state is a CRM request; tracking it would false-drift against okdctl's
  # out-of-band power ops.
  lifecycle {
    ignore_changes = [state]
  }
}

# rule is prefixed with cluster_name (PVE-cluster-wide id) so multiple okd
# clusters don't collide.
resource "proxmox_harule" "master_anti_affinity" {
  count = var.ha_enabled ? 1 : 0

  rule      = "${var.cluster_name}-master-anti-affinity"
  type      = "resource-affinity"
  affinity  = "negative"
  resources = [for r in proxmox_haresource.master : r.resource_id]
}
