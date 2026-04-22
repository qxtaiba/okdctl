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
  default     = "pve01"
}

variable "bridge" {
  description = "network bridge to use for vm network interfaces"
  type        = string
  default     = "vmbr0"
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
}

variable "data_storage" {
  description = "storage pool for data/ceph disks"
  type        = string
  default     = "local-lvm"
}

variable "bootstrap_iso" {
  description = "custom fedora coreos iso path for bootstrap node"
  type        = string
  default     = "local:iso/bootstrap.iso"
}

variable "master_isos" {
  description = "custom fedora coreos iso paths for master nodes"
  type        = list(string)
  default = [
    "local:iso/master0.iso",
    "local:iso/master1.iso",
    "local:iso/master2.iso"
  ]
}

variable "worker_isos" {
  description = "custom fedora coreos iso paths for worker nodes"
  type        = list(string)
  default = [
    "local:iso/worker0.iso",
    "local:iso/worker1.iso",
    "local:iso/worker2.iso"
  ]
}


# =============================================================================
# CLUSTER CONFIGURATION VARIABLES
# =============================================================================
variable "cluster_name" {
  description = "name of the okd cluster"
  type        = string
  default     = "grappleberry"
}

variable "vmid_base" {
  description = "base vm id for vms"
  type        = number
  default     = 7000
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
}

variable "worker_count" {
  description = "number of worker nodes to create"
  type        = number
  default     = 3
}


# =============================================================================
# VM RESOURCE CONFIGURATION
# =============================================================================
variable "cpu_cores" {
  description = "number of cpu cores per vm"
  type        = number
  default     = 4
}

variable "memory_mb" {
  description = "amount of memory in mb per vm"
  type        = number
  default     = 16384
}

variable "bootstrap_cpu_cores" {
  description = "cpu cores for bootstrap node (defaults to cpu_cores if not set)"
  type        = number
  default     = null
}

variable "bootstrap_memory_mb" {
  description = "memory for bootstrap node (defaults to memory_mb if not set)"
  type        = number
  default     = null
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
  default     = null
}

variable "worker_memory_mb" {
  description = "memory for worker nodes (defaults to memory_mb if not set)"
  type        = number
  default     = null
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

variable "additional_networks" {
  description = "additional network interfaces for vms"
  type = list(object({
    model  = string
    bridge = string
    tag    = optional(number)
  }))
  default = []
}

variable "numa_enabled" {
  description = "enable numa for vms"
  type        = bool
  default     = false
}

variable "vm_tags" {
  description = "tags to apply to all vms"
  type        = list(string)
  default     = []
}

variable "worker_data_disk_size_gb" {
  description = "size of data disk for worker nodes (0 = no data disk)"
  type        = number
  default     = 500
}

variable "master_data_disk_size_gb" {
  description = "size of data disk for master nodes (0 = no data disk)"
  type        = number
  default     = 0
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
  description = "Start worker nodes immediately on creation"
  type        = bool
  default     = false
}
