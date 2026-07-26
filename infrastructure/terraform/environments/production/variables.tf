# =============================================================================
# PROXMOX CONNECTION (set via environment variables)
# - PROXMOX_VE_ENDPOINT    (api url, e.g., https://pve.example.com:8006/)
# - PROXMOX_VE_USERNAME    (username, e.g., root@pam)
# - PROXMOX_VE_PASSWORD    (password)
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

# worker_count is exposed at the root so `okdctl node remove` / `add` can
# drive the worker VM set by count. Master count stays module-internal: master
# add/remove renumbers worker IPs and is guarded by prevent_destroy + the
# odd-quorum validator, so it is deliberately not a root knob.
variable "worker_count" {
  description = "number of worker nodes to create"
  type        = number
  default     = 3
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

# master_memory_mb / master_cpu_cores give `okdctl node resize masters` a clean
# per-role knob. Null falls back to memory_mb / cpu_cores in the module's
# coalesce, so a root that never sets them keeps the pre-widening behavior.
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

# master_data_disk_size_gb is exposed so the compaction runbook can give the
# masters Ceph OSD disks. Default 0 keeps masters diskless (matching how okdctl
# deploys today); the module omits the data disk entirely when this is 0.
variable "master_data_disk_size_gb" {
  description = "size of data disk for master nodes in gb (0 = no data disk)"
  type        = number
  default     = 0
}

# master_mon_disk_size_gb is exposed so the mon-disk runbook can give the
# masters a dedicated /var/lib/rook disk (scsi2). Default 0 keeps the disk
# absent; the module omits it entirely when this is 0.
variable "master_mon_disk_size_gb" {
  description = "size of dedicated mon-store disk for master nodes in gb (0 = no mon disk)"
  type        = number
  default     = 0
}


# =============================================================================
# NODE NAMES
# =============================================================================
# Exposed at the root so the rendered tfvars' cluster-prefixed names
# (${cluster}-masterN / ${cluster}-workerN) take effect instead of the module's
# bare masterN/workerN defaults. Without this, adopting the slim root would
# rename every existing VM in place. Root defaults mirror the module's so a
# standalone plan still validates; real deployments always supply names via
# tfvars.
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


# =============================================================================
# DEPLOY LIFECYCLE
# =============================================================================
# bootstrap_enabled and start_workers_immediately are the deploy-lifecycle knobs
# that node ops assert as post-deploy invariants: a running cluster has no
# bootstrap VM and its workers are started. Exposed at the root so `okdctl node`
# ops can pass them as -var overrides (defeating any stale terraform.tfvars
# value); defaults mirror the module, which the deploy flow flips via -var
# during install and cleanup.
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


# =============================================================================
# HIGH AVAILABILITY
# =============================================================================
variable "ha_enabled" {
  description = "enable proxmox ha anti-affinity for master vms (pve9+)"
  type        = bool
  default     = false
}
