# =============================================================================
# PROXMOX CONNECTION (set via environment variables)
# - PROXMOX_VE_ENDPOINT    (api url, e.g., https://pve.example.com:8006/)
# - PROXMOX_VE_USERNAME    (username, e.g., root@pam)
# - PROXMOX_VE_PASSWORD    (password)
# - PROXMOX_VE_INSECURE    (optional, set to "true" to disable tls verification)
# =============================================================================


# =============================================================================
# PROXMOX INFRASTRUCTURE VARIABLES
# =============================================================================

variable "target_node" {
  description = "proxmox node name where vms will be created"
  type        = string
}

variable "bridge" {
  description = "network bridge to use for vm network interfaces"
  type        = string
  default     = "vmbr0"
  validation {
    condition     = can(regex("^vmbr[0-9]+$", var.bridge))
    error_message = "bridge must be a valid proxmox bridge name (e.g., vmbr0, vmbr1)."
  }
}

variable "os_storage" {
  description = "storage pool for os disks"
  type        = string
  default     = "local-lvm"
}

variable "os_disk_size_gb" {
  description = "size of os disk in gb"
  type        = number
  default     = 50
  validation {
    condition     = var.os_disk_size_gb >= 20 && var.os_disk_size_gb <= 1000
    error_message = "os disk size must be between 20gb and 1000gb."
  }
}

variable "master_os_disk_size_gb" {
  description = "os disk size for master nodes (defaults to os_disk_size_gb)"
  type        = number
  default     = null
  validation {
    condition     = var.master_os_disk_size_gb == null ? true : (var.master_os_disk_size_gb >= 20 && var.master_os_disk_size_gb <= 1000)
    error_message = "master os disk size must be between 20gb and 1000gb."
  }
}

variable "worker_os_disk_size_gb" {
  description = "os disk size for worker nodes (defaults to os_disk_size_gb)"
  type        = number
  default     = null
  validation {
    condition     = var.worker_os_disk_size_gb == null ? true : (var.worker_os_disk_size_gb >= 20 && var.worker_os_disk_size_gb <= 1000)
    error_message = "worker os disk size must be between 20gb and 1000gb."
  }
}

variable "data_storage" {
  description = "storage pool for data/ceph disks"
  type        = string
  default     = "local-lvm"
}

variable "minimum_data_disk_size_gb" {
  description = "floor for data-disk activation; setting to 1 prevents a re-apply that zeros master_data_disk_size_gb or worker_data_disk_size_gb from silently destroying existing disks"
  type        = number
  default     = 0
  validation {
    condition     = var.minimum_data_disk_size_gb >= 0
    error_message = "minimum_data_disk_size_gb must be >= 0."
  }
}

variable "worker_data_disk_size_gb" {
  description = "size of data disk for worker nodes in gb; 0 (or any value below minimum_data_disk_size_gb) omits the disk — lowering this after initial apply destroys the ceph data disk"
  type        = number
  default     = 500
}

variable "master_data_disk_size_gb" {
  description = "size of data disk for master nodes in gb; 0 (or any value below minimum_data_disk_size_gb) omits the disk — lowering this after initial apply destroys the ceph data disk"
  type        = number
  default     = 0
}

variable "bootstrap_iso" {
  description = "custom fedora coreos iso path for bootstrap node"
  type        = string
}

variable "master_isos" {
  description = "custom fedora coreos iso paths for master nodes"
  type        = list(string)
  validation {
    condition     = length(var.master_isos) >= 1
    error_message = "at least one master iso must be provided."
  }
}

variable "worker_isos" {
  description = "custom fedora coreos iso paths for worker nodes"
  type        = list(string)
  validation {
    condition     = length(var.worker_isos) >= 1
    error_message = "at least one worker iso must be provided."
  }
}


# =============================================================================
# CLUSTER CONFIGURATION VARIABLES
# =============================================================================

variable "cluster_name" {
  description = "name of the okd cluster"
  type        = string
  default     = "okd"
  validation {
    condition     = can(regex("^[a-z0-9-]+$", var.cluster_name))
    error_message = "cluster name must contain only lowercase letters, numbers, and hyphens."
  }
}

variable "vmid_base" {
  description = "base vm id for vms (bootstrap=base, masters=base+10+n, workers=base+100+n)"
  type        = number
  default     = 6000
  validation {
    condition     = var.vmid_base >= 100 && var.vmid_base <= 9000
    error_message = "vm id base must be between 100 and 9000 to allow for node numbering."
  }
}

variable "bootstrap_enabled" {
  description = "whether to create bootstrap node"
  type        = bool
  default     = true
}

variable "master_count" {
  description = "number of master nodes to create"
  type        = number
  default     = 3
  validation {
    condition     = var.master_count >= 1 && var.master_count <= 5 && var.master_count % 2 == 1
    error_message = "master count must be an odd number between 1 and 5 for ha."
  }
}

variable "worker_count" {
  description = "number of worker nodes to create"
  type        = number
  default     = 3
  validation {
    condition     = var.worker_count >= 0 && var.worker_count <= 20
    error_message = "worker count must be between 0 and 20."
  }
}


# =============================================================================
# VM RESOURCE CONFIGURATION
# =============================================================================

variable "cpu_cores" {
  description = "number of cpu cores per vm"
  type        = number
  default     = 4
  validation {
    condition     = var.cpu_cores >= 2 && var.cpu_cores <= 32
    error_message = "cpu cores must be between 2 and 32."
  }
}

variable "memory_mb" {
  description = "amount of memory in mb per vm"
  type        = number
  default     = 12288
  validation {
    condition     = var.memory_mb >= 8192
    error_message = "memory must be at least 8gb (8192mb) for okd nodes."
  }
}

# optional: different resources per node type
variable "bootstrap_cpu_cores" {
  description = "cpu cores for bootstrap node (defaults to cpu_cores if not set)"
  type        = number
  default     = null
}

variable "bootstrap_memory_mb" {
  description = "memory for bootstrap node (defaults to memory_mb if not set)"
  type        = number
  default     = 8192
}

variable "master_cpu_cores" {
  description = "cpu cores for master nodes (defaults to cpu_cores if not set)"
  type        = number
  default     = null
}

variable "master_memory_mb" {
  description = "memory for master nodes (defaults to memory_mb if not set)"
  type        = number
  default     = null
}

variable "worker_cpu_cores" {
  description = "cpu cores for worker nodes (defaults to cpu_cores if not set)"
  type        = number
  default     = 8
}

variable "worker_memory_mb" {
  description = "memory for worker nodes (defaults to memory_mb if not set)"
  type        = number
  default     = 20480
}


# =============================================================================
# NODE NAMES
# =============================================================================

variable "master_names" {
  description = "list of master node names"
  type        = list(string)
  default     = ["master0", "master1", "master2"]
}

variable "worker_names" {
  description = "list of worker node names"
  type        = list(string)
  default     = ["worker0", "worker1", "worker2"]
}


# =============================================================================
# IGNITION AND NETWORK CONFIGURATION
# =============================================================================

# variable "bootstrap_kernel_args" {
#   description = "kernel arguments for bootstrap node including ignition url and network config"
#   type        = string
# }

# variable "master_kernel_args" {
#   description = "list of kernel arguments for master nodes (one per master)"
#   type        = list(string)
# }

# variable "worker_kernel_args" {
#   description = "list of kernel arguments for worker nodes (one per worker)"
#   type        = list(string)
# }


# =============================================================================
# OPTIONAL CONFIGURATION
# =============================================================================

variable "vm_tags" {
  description = "tags to apply to all vms"
  type        = list(string)
  default     = ["okd", "kubernetes"]
}

variable "additional_networks" {
  description = "additional network interfaces for vms"
  type = list(object({
    model  = string
    bridge = string
    tag    = optional(number)
  }))
  default = []
  validation {
    condition     = alltrue([for net in var.additional_networks : net.tag == null || (net.tag >= 1 && net.tag <= 4094)])
    error_message = "vlan tags must be between 1 and 4094."
  }
}

variable "numa_enabled" {
  description = "enable numa for vms"
  type        = bool
  default     = false
}

variable "cpu_type" {
  description = "cpu type for vms (host, x86-64-v2, x86-64-v3, kvm64)"
  type        = string
  default     = "host"
}

variable "master_target_nodes" {
  description = "per-master proxmox node assignment (index-based, falls back to target_node)"
  type        = list(string)
  default     = []
}

variable "worker_target_nodes" {
  description = "per-worker proxmox node assignment (index-based, falls back to target_node)"
  type        = list(string)
  default     = []
}

variable "start_workers_immediately" {
  description = "Start worker nodes immediately on creation (false to start after bootstrap)"
  type        = bool
  default     = false # Default to delayed start for reliability
}
