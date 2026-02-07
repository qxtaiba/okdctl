# =============================================================================
# PROVIDER CONFIGURATION
# =============================================================================

# uses environment variables:
# - PROXMOX_VE_ENDPOINT    (api url, e.g., https://pve.example.com:8006/)
# - PROXMOX_VE_USERNAME    (username, e.g., root@pam)
# - PROXMOX_VE_PASSWORD    (password)
# - PROXMOX_VE_INSECURE    (optional, set to "true" to disable tls verification)
# Provider configuration is handled by the parent module

# =============================================================================
# LOCALS
# =============================================================================

locals {
  # node lists based on counts
  masters = slice(var.master_names, 0, var.master_count)
  workers = slice(var.worker_names, 0, var.worker_count)

  # resource configurations with fallbacks
  bootstrap_cpu    = coalesce(var.bootstrap_cpu_cores, var.cpu_cores)
  bootstrap_memory = coalesce(var.bootstrap_memory_mb, var.memory_mb)
  master_cpu       = coalesce(var.master_cpu_cores, var.cpu_cores)
  master_memory    = coalesce(var.master_memory_mb, var.memory_mb)
  worker_cpu       = coalesce(var.worker_cpu_cores, var.cpu_cores)
  worker_memory    = coalesce(var.worker_memory_mb, var.memory_mb)
}

# =============================================================================
# BOOTSTRAP NODE
# =============================================================================

resource "proxmox_virtual_environment_vm" "bootstrap" {
  count = var.bootstrap_enabled ? 1 : 0

  # vm identity
  name      = "${var.cluster_name}-bootstrap"
  node_name = var.target_node
  vm_id     = var.vmid_base

  # disable auto-start on proxmox host boot
  on_boot = false

  # cpu configuration
  cpu {
    cores   = local.bootstrap_cpu
    sockets = 1
    type    = "host"
  }

  # memory configuration
  memory {
    dedicated = local.bootstrap_memory
  }

  # agent configuration - disabled for faster terraform operations
  agent {
    enabled = false
  }

  # stop vm before destroy (faster than waiting for graceful shutdown)
  stop_on_destroy = true

  # scsi hardware for scsi disks
  scsi_hardware = "virtio-scsi-single"

  # boot configuration
  startup {
    order      = 1
    up_delay   = 30
    down_delay = 120
  }

  # os disk
  disk {
    datastore_id = var.os_storage
    size         = var.os_disk_size_gb
    interface    = "scsi0"
    iothread     = true
    ssd          = false
    discard      = "on"
    serial       = "OS-DISK"
  }

  # iso mount
  cdrom {
    file_id   = var.bootstrap_iso
    interface = "ide2"
  }

  # primary network
  network_device {
    bridge = var.bridge
    model  = "virtio"
  }

  # additional networks if specified
  dynamic "network_device" {
    for_each = var.additional_networks
    content {
      bridge  = network_device.value.bridge
      model   = network_device.value.model
      vlan_id = network_device.value.tag
    }
  }

  # tags
  tags = var.vm_tags

  # description
  description = "OKD Bootstrap Node for cluster ${var.cluster_name}"

  # operating system and boot configuration
  operating_system {
    type = "l26" # linux kernel 2.6+
  }

  # q35 machine type for better compatibility
  machine = "q35"

  # uefi bios configuration
  bios = "ovmf"

  # boot configuration - disk first, then iso
  boot_order = ["scsi0", "ide2", "net0"]

  # efi disk required for uefi boot
  efi_disk {
    datastore_id = var.os_storage
    type         = "4m"
  }


  lifecycle {
    ignore_changes = [
      network_device,
      startup,
      cdrom,
      boot_order,
    ]
  }
}

# =============================================================================
# MASTER NODES
# =============================================================================

resource "proxmox_virtual_environment_vm" "master" {
  count = length(local.masters)

  # vm identity (names already include cluster prefix from provisioner)
  name      = local.masters[count.index]
  node_name = var.target_node
  vm_id     = var.vmid_base + 10 + count.index

  # disable auto-start on proxmox host boot
  on_boot = false

  # cpu configuration
  cpu {
    cores   = local.master_cpu
    sockets = 1
    type    = "host"
  }

  # memory configuration
  memory {
    dedicated = local.master_memory
  }

  # agent configuration - disabled for faster terraform operations
  agent {
    enabled = false
  }

  # stop vm before destroy (faster than waiting for graceful shutdown)
  stop_on_destroy = true

  # scsi hardware for scsi disks
  scsi_hardware = "virtio-scsi-single"

  # boot configuration
  startup {
    order      = 2
    up_delay   = 30
    down_delay = 120
  }

  # os disk
  disk {
    datastore_id = var.os_storage
    size         = var.os_disk_size_gb
    interface    = "scsi0"
    iothread     = true
    ssd          = false
    discard      = "on"
    serial       = "OS-DISK"
  }

  # iso mount
  cdrom {
    file_id   = var.master_isos[count.index]
    interface = "ide2"
  }

  # primary network
  network_device {
    bridge = var.bridge
    model  = "virtio"
  }

  # additional networks if specified
  dynamic "network_device" {
    for_each = var.additional_networks
    content {
      bridge  = network_device.value.bridge
      model   = network_device.value.model
      vlan_id = network_device.value.tag
    }
  }

  # tags
  tags = var.vm_tags

  # description
  description = "OKD Master Node ${local.masters[count.index]} for cluster ${var.cluster_name}"

  # operating system and boot configuration
  operating_system {
    type = "l26"
  }

  # q35 machine type for better compatibility
  machine = "q35"

  # uefi bios configuration
  bios = "ovmf"

  # boot configuration - disk first, then iso
  boot_order = ["scsi0", "ide2", "net0"]

  # efi disk required for uefi boot
  efi_disk {
    datastore_id = var.os_storage
    type         = "4m"
  }


  depends_on = [proxmox_virtual_environment_vm.bootstrap]

  lifecycle {
    ignore_changes = [
      network_device,
      startup,
      cdrom,
      boot_order,
    ]
  }
}

# =============================================================================
# WORKER NODES
# =============================================================================

resource "proxmox_virtual_environment_vm" "worker" {
  count = length(local.workers)

  # vm identity (names already include cluster prefix from provisioner)
  name      = local.workers[count.index]
  node_name = var.target_node
  vm_id     = var.vmid_base + 100 + count.index

  # disable auto-start on proxmox host boot
  on_boot = false

  # control whether worker starts immediately on creation
  # set to false to delay worker start until after bootstrap completes
  started = var.start_workers_immediately

  # cpu configuration
  cpu {
    cores   = local.worker_cpu
    sockets = 1
    type    = "host"
  }

  # memory configuration
  memory {
    dedicated = local.worker_memory
  }

  # agent configuration - disabled for faster terraform operations
  agent {
    enabled = false
  }

  # stop vm before destroy (faster than waiting for graceful shutdown)
  stop_on_destroy = true

  # scsi hardware for scsi disks
  scsi_hardware = "virtio-scsi-single"

  # boot configuration
  startup {
    order      = 3
    up_delay   = 30
    down_delay = 120
  }

  # os disk
  disk {
    datastore_id = var.os_storage
    size         = var.os_disk_size_gb
    interface    = "scsi0"
    iothread     = true
    ssd          = false
    discard      = "on"
    serial       = "OS-DISK"
  }

  # data disk (ceph osd)
  disk {
    datastore_id = var.data_storage
    size         = var.data_disk_size_gb
    interface    = "scsi1"
    iothread     = true
    ssd          = false
    discard      = "on"
    serial       = "CEPH-DATA"
  }

  # iso mount
  cdrom {
    file_id   = var.worker_isos[count.index]
    interface = "ide2"
  }

  # primary network
  network_device {
    bridge = var.bridge
    model  = "virtio"
  }

  # additional networks if specified
  dynamic "network_device" {
    for_each = var.additional_networks
    content {
      bridge  = network_device.value.bridge
      model   = network_device.value.model
      vlan_id = network_device.value.tag
    }
  }

  # tags
  tags = var.vm_tags

  # description
  description = "OKD Worker Node ${local.workers[count.index]} for cluster ${var.cluster_name}"

  # operating system and boot configuration
  operating_system {
    type = "l26"
  }

  # q35 machine type for better compatibility
  machine = "q35"

  # uefi bios configuration
  bios = "ovmf"

  # boot configuration - disk first, then iso
  boot_order = ["scsi0", "ide2", "net0"]

  # efi disk required for uefi boot
  efi_disk {
    datastore_id = var.os_storage
    type         = "4m"
  }


  depends_on = [proxmox_virtual_environment_vm.master]

  lifecycle {
    ignore_changes = [
      network_device,
      startup,
      cdrom,
      boot_order,
    ]
  }
}
