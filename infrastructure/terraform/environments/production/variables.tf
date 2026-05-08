# =============================================================================
# PROXMOX CONNECTION (set via environment variables)
# - PROXMOX_VE_ENDPOINT    (api url, e.g., https://pve.example.com:8006/)
# - PROXMOX_VE_USERNAME    (username, e.g., root@pam)
# - PROXMOX_VE_PASSWORD    (password)
# - PROXMOX_VE_INSECURE    (optional, set to "true" to disable tls verification)
# - PROXMOX_VE_INSECURE  (DEV ONLY: disables TLS verification — never set in prod; use a CA-signed cert or add the proxmox CA to your trust store)
# =============================================================================


# =============================================================================
# PROXMOX INFRASTRUCTURE VARIABLES
# =============================================================================
variable "target_node" {
  description = "proxmox node name where vms will be created"
  type        = string
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
}

variable "vmid_base" {
  description = "base vm id for vms"
  type        = number
}


# =============================================================================
# VM RESOURCE CONFIGURATION
# =============================================================================
# Variables identical to module defaults are intentionally omitted so the
# module's own validation blocks (cpu_cores 2-32, memory_mb >= 8192,
# master_count odd 1-5, vmid_base 100-9000, etc.) are the single source of
# truth. Only env-specific overrides remain below.
variable "memory_mb" {
  description = "amount of memory in mb per vm"
  type        = number
  default     = 16384
}

variable "bootstrap_memory_mb" {
  description = "memory for bootstrap node (defaults to memory_mb if not set)"
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

variable "vm_tags" {
  description = "tags to apply to all vms"
  type        = list(string)
  default     = []
}
