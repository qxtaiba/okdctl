# proxmox connection (env vars): PROXMOX_VE_ENDPOINT, PROXMOX_VE_USERNAME, PROXMOX_VE_PASSWORD.
# PROXMOX_VE_INSECURE disables TLS verification — dev only, never set in production.

# Every var the tfvars template renders must be declared + wired in main.tf,
# else terraform silently uses the module default (TestTfvarsTemplateVarsWired).
# Defaults mirror the module; validation stays module-only.


variable "target_node" {
  description = "proxmox node name where vms will be created"
  type        = string
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


variable "cluster_name" {
  description = "name of the okd cluster"
  type        = string
}

variable "vmid_base" {
  description = "base vm id for vms"
  type        = number
}

# Passthrough only — node ops never override master_count; add/remove is
# unsupported (guarded by prevent_destroy + the module's odd-quorum validator).
variable "master_count" {
  description = "number of master nodes to create"
  type        = number
  default     = 3
}

# Exposed at the root so `okdctl node remove`/`add` can drive the worker VM set by count.
variable "worker_count" {
  description = "number of worker nodes to create"
  type        = number
  default     = 3
}


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

variable "worker_cpu_cores" {
  description = "cpu cores for worker nodes (defaults to cpu_cores if not set)"
  type        = number
  default     = null
}

# Gives `okdctl node resize masters` a per-role knob; null falls back to
# memory_mb/cpu_cores in the module's coalesce, preserving pre-widening behavior.
variable "master_memory_mb" {
  description = "memory for master nodes (defaults to memory_mb if not set)"
  type        = number
  default     = null
}

variable "master_cpu_cores" {
  description = "cpu cores for master nodes (defaults to cpu_cores if not set)"
  type        = number
  default     = null
}

variable "worker_memory_mb" {
  description = "memory for worker nodes (defaults to memory_mb if not set)"
  type        = number
  default     = null
}

variable "os_disk_size_gb" {
  description = "size of os disk in gb"
  type        = number
  default     = 50
}

variable "master_os_disk_size_gb" {
  description = "os disk size for master nodes (defaults to os_disk_size_gb)"
  type        = number
  default     = null
}

variable "worker_os_disk_size_gb" {
  description = "os disk size for worker nodes (defaults to os_disk_size_gb)"
  type        = number
  default     = null
}

variable "worker_data_disk_size_gb" {
  description = "size of data disk for worker nodes in gb (0 = no data disk)"
  type        = number
  default     = 500
}

# Exposed for the compaction runbook to give masters Ceph OSD disks; default 0
# keeps masters diskless — the module omits the data disk entirely when 0.
variable "master_data_disk_size_gb" {
  description = "size of data disk for master nodes in gb (0 = no data disk)"
  type        = number
  default     = 0
}

# Exposed for the mon-disk runbook to give masters a dedicated /var/lib/rook
# disk (scsi2); default 0 omits the disk entirely.
variable "master_mon_disk_size_gb" {
  description = "size of dedicated mon-store disk for master nodes in gb (0 = no mon disk)"
  type        = number
  default     = 0
}

# Set to the smallest data-disk size in use (e.g. 500) after initial apply —
# plan fails if a regenerated tfvars/typo drops a nonzero size below this floor.
# Zeroing still removes the disk: terraform can't tell "never had one" from "being deleted".
variable "minimum_data_disk_size_gb" {
  description = "plan-time floor for nonzero data-disk sizes (0 disables the guard)"
  type        = number
  default     = 0
}


# Exposed at the root so tfvars' cluster-prefixed names take effect instead of
# the module's bare defaults — needed so adopting the slim root doesn't rename existing VMs.
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

variable "vm_tags" {
  description = "tags to apply to all vms"
  type        = list(string)
  default     = []
}


# Deploy-lifecycle knobs node ops assert as post-deploy invariants (no bootstrap
# VM, workers started); exposed at the root so `okdctl node` can -var override them.
variable "bootstrap_enabled" {
  description = "whether to create the bootstrap node"
  type        = bool
  default     = true
}

variable "start_workers_immediately" {
  description = "start worker vms on create instead of delaying until the control plane is ready"
  type        = bool
  default     = false
}


variable "ha_enabled" {
  description = "enable proxmox ha anti-affinity for master vms (pve9+)"
  type        = bool
  default     = false
}


variable "cpu_type" {
  description = "qemu cpu model for vms (commonly host, x86-64-v2, x86-64-v3, or kvm64; any model plus flags accepted)"
  type        = string
  default     = "host"
}

variable "numa_enabled" {
  description = "enable numa for vms"
  type        = bool
  default     = false
}

variable "additional_networks" {
  description = "additional network interfaces for vms"
  type = list(object({
    model  = string
    bridge = string
    tag    = optional(number)
  }))
  default = []
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
