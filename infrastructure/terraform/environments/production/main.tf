# =============================================================================
# OKD CLUSTER DEPLOYMENT ON PROXMOX - PRODUCTION ENVIRONMENT
# =============================================================================

module "okd_cluster" {
  source = "../../modules/proxmox-okd"

  # =============================================================================
  # PROXMOX INFRASTRUCTURE
  # =============================================================================

  target_node     = var.target_node
  bridge          = var.bridge
  os_storage      = var.os_storage
  data_storage    = var.data_storage
  os_disk_size_gb = var.os_disk_size_gb
  bootstrap_iso   = var.bootstrap_iso
  master_isos     = var.master_isos
  worker_isos     = var.worker_isos

  # =============================================================================
  # CLUSTER CONFIGURATION
  # =============================================================================

  cluster_name      = var.cluster_name
  vmid_base         = var.vmid_base
  bootstrap_enabled = var.bootstrap_enabled
  master_count      = var.master_count
  worker_count      = var.worker_count

  # =============================================================================
  # VM RESOURCES
  # =============================================================================

  cpu_cores = var.cpu_cores
  memory_mb = var.memory_mb

  bootstrap_cpu_cores = var.bootstrap_cpu_cores
  bootstrap_memory_mb = var.bootstrap_memory_mb
  master_cpu_cores    = var.master_cpu_cores
  master_memory_mb    = var.master_memory_mb
  worker_cpu_cores    = var.worker_cpu_cores
  worker_memory_mb    = var.worker_memory_mb

  # =============================================================================
  # NODE NAMES
  # =============================================================================

  master_names = var.master_names
  worker_names = var.worker_names

  # =============================================================================
  # IGNITION CONFIGURATION
  # =============================================================================

  # bootstrap_kernel_args = var.bootstrap_kernel_args
  # master_kernel_args    = var.master_kernel_args
  # worker_kernel_args    = var.worker_kernel_args

  # =============================================================================
  # OPTIONAL CONFIGURATION
  # =============================================================================

  worker_data_disk_size_gb = var.worker_data_disk_size_gb
  master_data_disk_size_gb = var.master_data_disk_size_gb
  cpu_type                 = var.cpu_type
  master_target_nodes      = var.master_target_nodes
  worker_target_nodes      = var.worker_target_nodes

  vm_tags             = var.vm_tags
  additional_networks = var.additional_networks
  numa_enabled        = var.numa_enabled

  start_workers_immediately = var.start_workers_immediately
}
