# proxmox connection (set via environment variables)
# - PROXMOX_VE_ENDPOINT    (api url, e.g., https://pve.example.com:8006/)
# - PROXMOX_VE_USERNAME    (username, e.g., root@pam)
# - PROXMOX_VE_PASSWORD    (password)
# - PROXMOX_VE_INSECURE  (DEV ONLY: disables TLS verification — never set in prod; use a CA-signed cert or add the proxmox CA to your trust store)

# Every variable okdctl's terraform.tfvars template renders must be declared
# here and passed to the module in main.tf — terraform silently ignores tfvars
# values for undeclared variables, so a missing declaration means the module
# default wins over user config (TestTfvarsTemplateVarsWired pins this).
# Defaults mirror the module's; validation blocks stay module-only so the
# module remains the single source of truth for constraints.


# proxmox infrastructure variables
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


# cluster configuration variables
variable "cluster_name" {
  description = "name of the okd cluster"
  type        = string
}

variable "vmid_base" {
  description = "base vm id for vms"
  type        = number
}

# master_count only passes the rendered tfvars value through (compact
# single-master clusters); node ops never override it via -var — master
# add/remove stays unsupported, guarded by prevent_destroy + the module's
# odd-quorum validator.
variable "master_count" {
  description = "number of master nodes to create"
  type        = number
  default     = 3
}

# worker_count is exposed at the root so `okdctl node remove` / `add` can
# drive the worker VM set by count.
variable "worker_count" {
  description = "number of worker nodes to create"
  type        = number
  default     = 3
}


# vm resource configuration
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

# Set this to the smallest data-disk size in use (e.g. 500) after initial
# apply so a regenerated tfvars or a typo cannot shrink a ceph disk: the
# module fails the plan when a nonzero size drops below this floor. Zeroing
# a size still removes the disk — terraform cannot tell "never had a disk"
# from "disk being deleted".
variable "minimum_data_disk_size_gb" {
  description = "plan-time floor for nonzero data-disk sizes (0 disables the guard)"
  type        = number
  default     = 0
}


# node names
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


# deploy lifecycle
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


# high availability
variable "ha_enabled" {
  description = "enable proxmox ha anti-affinity for master vms (pve9+)"
  type        = bool
  default     = false
}


# hardware and placement
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
