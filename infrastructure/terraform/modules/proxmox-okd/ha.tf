# high availability (opt-in, ha_enabled)
# Provisions Proxmox HA-manager resources for the master VMs declared in
# main.tf so a PVE-node failure relocates masters onto surviving nodes
# instead of leaving them down until manual intervention, with anti-affinity
# keeping masters spread across nodes.
#
# Interaction with main.tf's per-VM startup{} block: startup{} (order,
# up_delay, down_delay) is enforced by pvestatd on local node boot only and
# always applies. Once a resource is added to the HA manager (this file),
# HA supersedes startup{} for anything HA-triggered — a failover relocation
# does not replay startup{} ordering on the target node.
#
# Interaction with okdctl's own power management (cluster stop/start, resize
# power-cycle, snapshot rollback): the HA request-state below starts as
# "started", and the CRM enforces the request-state, not terraform. When
# okdctl powers a VM off out-of-band the request-state diverges;
# ignore_changes on state keeps that divergence out of `okdctl plan` (no
# false drift) and out of the next apply (which would otherwise power a
# deliberately-stopped master back on). It does NOT stop the CRM itself from
# reacting to an out-of-band shutdown — that interaction is unverified on a
# live PVE9 cluster, which is why okdctl cluster stop/start warn when
# ha_enabled is set.

data "proxmox_version" "current" {
  count = var.ha_enabled ? 1 : 0

  lifecycle {
    postcondition {
      condition     = tonumber(split(".", self.release)[0]) >= 9
      error_message = "proxmox ha anti-affinity (harule resource-affinity) requires pve 9+; detected release ${self.release}"
    }
  }
}

# The for_each value dereferences the master VM resource, so every haresource
# instance records a whole-resource dependency on every master (masters use
# count, not for_each — no per-instance edge exists). A master-scoped
# terraform destroy therefore cannot cleanly drop one master's HA membership:
# it either fans the removal across all masters or orphans the vm:<id> CRM
# entry, and neither shows in a scoped plan. prevent_destroy on the master
# resource blocks that path until an operator applies the override escape
# hatch (see main.tf). A clean per-instance edge needs masters migrated from
# count to for_each — a state-address change tracked for audit-state-and-recovery.
resource "proxmox_haresource" "master" {
  for_each = var.ha_enabled ? { for idx, name in local.masters : name => proxmox_virtual_environment_vm.master[idx].vm_id } : {}

  depends_on = [data.proxmox_version.current]

  resource_id = "vm:${each.value}"
  state       = "started"

  # state is a REQUEST to the CRM, and okdctl legitimately changes the live
  # power state out-of-band (cluster stop, resize power-cycle, snapshot
  # rollback). Tracking it would report every stopped cluster as drift and
  # make the next apply power deliberately-stopped masters back on.
  lifecycle {
    ignore_changes = [state]
  }
}

# rule is prefixed with cluster_name because it is a PVE-cluster-wide
# identifier, not scoped to this terraform state — multiple okd clusters on
# the same proxmox cluster must not collide on the same ha rule name.
resource "proxmox_harule" "master_anti_affinity" {
  count = var.ha_enabled ? 1 : 0

  rule      = "${var.cluster_name}-master-anti-affinity"
  type      = "resource-affinity"
  affinity  = "negative"
  resources = [for r in proxmox_haresource.master : r.resource_id]
}
