# =============================================================================
# OKD CLUSTER DEPLOYMENT ON PROXMOX - PRODUCTION ENVIRONMENT
# =============================================================================

module "okd_cluster" {
  source = "../../modules/proxmox-okd"

  # =============================================================================
  # PROXMOX INFRASTRUCTURE
  # =============================================================================

  target_node   = var.target_node
  bootstrap_iso = var.bootstrap_iso
  master_isos   = var.master_isos
  worker_isos   = var.worker_isos

  # =============================================================================
  # CLUSTER CONFIGURATION
  # =============================================================================

  cluster_name = var.cluster_name
  vmid_base    = var.vmid_base
  worker_count = var.worker_count
  master_names = var.master_names
  worker_names = var.worker_names

  # =============================================================================
  # VM RESOURCES (env-specific overrides only; module owns defaults+validation)
  # =============================================================================

  memory_mb                = var.memory_mb
  bootstrap_memory_mb      = var.bootstrap_memory_mb
  worker_cpu_cores         = var.worker_cpu_cores
  worker_memory_mb         = var.worker_memory_mb
  master_cpu_cores         = var.master_cpu_cores
  master_memory_mb         = var.master_memory_mb
  master_data_disk_size_gb = var.master_data_disk_size_gb

  vm_tags = var.vm_tags
}
