# =============================================================================
# CLUSTER INFORMATION
# =============================================================================

output "cluster_name" {
  description = "name of the okd cluster"
  value       = var.cluster_name
}

output "cluster_nodes_total" {
  description = "total number of nodes in the cluster"
  value       = var.master_count + var.worker_count + (var.bootstrap_enabled ? 1 : 0)
}

# =============================================================================
# BOOTSTRAP NODE OUTPUTS
# =============================================================================

output "bootstrap_vm_id" {
  description = "vm id of the bootstrap node"
  value       = var.bootstrap_enabled ? proxmox_virtual_environment_vm.bootstrap[0].vm_id : null
}

output "bootstrap_vm_name" {
  description = "vm name of the bootstrap node"
  value       = var.bootstrap_enabled ? proxmox_virtual_environment_vm.bootstrap[0].name : null
}

output "bootstrap_node_info" {
  description = "bootstrap node information"
  value = var.bootstrap_enabled ? {
    name   = proxmox_virtual_environment_vm.bootstrap[0].name
    vm_id  = proxmox_virtual_environment_vm.bootstrap[0].vm_id
    cores  = proxmox_virtual_environment_vm.bootstrap[0].cpu[0].cores
    memory = proxmox_virtual_environment_vm.bootstrap[0].memory[0].dedicated
  } : null
}

# =============================================================================
# MASTER NODE OUTPUTS
# =============================================================================

output "master_vm_ids" {
  description = "list of vm ids for master nodes"
  value       = [for master in proxmox_virtual_environment_vm.master : master.vm_id]
}

output "master_vm_names" {
  description = "list of vm names for master nodes"
  value       = [for master in proxmox_virtual_environment_vm.master : master.name]
}

output "master_nodes_info" {
  description = "master nodes information"
  value = [
    for master in proxmox_virtual_environment_vm.master : {
      name   = master.name
      vm_id  = master.vm_id
      cores  = master.cpu[0].cores
      memory = master.memory[0].dedicated
    }
  ]
}

# =============================================================================
# WORKER NODE OUTPUTS
# =============================================================================

output "worker_vm_ids" {
  description = "list of vm ids for worker nodes"
  value       = [for worker in proxmox_virtual_environment_vm.worker : worker.vm_id]
}

output "worker_vm_names" {
  description = "list of vm names for worker nodes"
  value       = [for worker in proxmox_virtual_environment_vm.worker : worker.name]
}

output "worker_nodes_info" {
  description = "worker nodes information"
  value = [
    for worker in proxmox_virtual_environment_vm.worker : {
      name   = worker.name
      vm_id  = worker.vm_id
      cores  = worker.cpu[0].cores
      memory = worker.memory[0].dedicated
    }
  ]
}

# =============================================================================
# ALL NODES SUMMARY
# =============================================================================

output "all_vm_ids" {
  description = "list of all vm ids in the cluster"
  value = concat(
    var.bootstrap_enabled ? [proxmox_virtual_environment_vm.bootstrap[0].vm_id] : [],
    [for master in proxmox_virtual_environment_vm.master : master.vm_id],
    [for worker in proxmox_virtual_environment_vm.worker : worker.vm_id]
  )
}

output "all_vm_names" {
  description = "list of all vm names in the cluster"
  value = concat(
    var.bootstrap_enabled ? [proxmox_virtual_environment_vm.bootstrap[0].name] : [],
    [for master in proxmox_virtual_environment_vm.master : master.name],
    [for worker in proxmox_virtual_environment_vm.worker : worker.name]
  )
}

output "all_nodes_info" {
  description = "all nodes information grouped by role"
  value = {
    bootstrap = var.bootstrap_enabled ? {
      name   = proxmox_virtual_environment_vm.bootstrap[0].name
      vm_id  = proxmox_virtual_environment_vm.bootstrap[0].vm_id
      cores  = proxmox_virtual_environment_vm.bootstrap[0].cpu[0].cores
      memory = proxmox_virtual_environment_vm.bootstrap[0].memory[0].dedicated
    } : null
    masters = [
      for master in proxmox_virtual_environment_vm.master : {
        name   = master.name
        vm_id  = master.vm_id
        cores  = master.cpu[0].cores
        memory = master.memory[0].dedicated
      }
    ]
    workers = [
      for worker in proxmox_virtual_environment_vm.worker : {
        name   = worker.name
        vm_id  = worker.vm_id
        cores  = worker.cpu[0].cores
        memory = worker.memory[0].dedicated
      }
    ]
  }
}

# =============================================================================
# RESOURCE SUMMARY
# =============================================================================

output "cluster_resources" {
  description = "summary of cluster resources"
  value = {
    total_cpus = (
      (var.bootstrap_enabled ? coalesce(var.bootstrap_cpu_cores, var.cpu_cores) : 0) +
      (coalesce(var.master_cpu_cores, var.cpu_cores) * var.master_count) +
      (coalesce(var.worker_cpu_cores, var.cpu_cores) * var.worker_count)
    )
    total_memory_gb = (
      (var.bootstrap_enabled ? coalesce(var.bootstrap_memory_mb, var.memory_mb) : 0) +
      (coalesce(var.master_memory_mb, var.memory_mb) * var.master_count) +
      (coalesce(var.worker_memory_mb, var.memory_mb) * var.worker_count)
    ) / 1024
    total_storage_gb = (
      (var.bootstrap_enabled ? var.os_disk_size_gb : 0) +
      (local.master_os_disk * var.master_count) +
      ((local.worker_os_disk + var.data_disk_size_gb) * var.worker_count)
    )
  }
}

# =============================================================================
# VM ID RANGES (USEFUL FOR NETWORKING/FIREWALL RULES)
# =============================================================================

output "vm_id_ranges" {
  description = "vm id ranges for different node types"
  value = {
    bootstrap_range = var.bootstrap_enabled ? "${var.vmid_base}-${var.vmid_base}" : null
    master_range    = var.master_count > 0 ? "${var.vmid_base + 10}-${var.vmid_base + 10 + var.master_count - 1}" : null
    worker_range    = var.worker_count > 0 ? "${var.vmid_base + 100}-${var.vmid_base + 100 + var.worker_count - 1}" : null
  }
}
