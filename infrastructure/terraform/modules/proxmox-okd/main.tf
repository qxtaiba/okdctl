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
  masters = slice(var.master_names, 0, var.master_count)
  workers = slice(var.worker_names, 0, var.worker_count)

  bootstrap_cpu    = coalesce(var.bootstrap_cpu_cores, var.cpu_cores)
  bootstrap_memory = coalesce(var.bootstrap_memory_mb, var.memory_mb)
  master_cpu       = coalesce(var.master_cpu_cores, var.cpu_cores)
  master_memory    = coalesce(var.master_memory_mb, var.memory_mb)
  master_os_disk   = coalesce(var.master_os_disk_size_gb, var.os_disk_size_gb)
  worker_cpu       = coalesce(var.worker_cpu_cores, var.cpu_cores)
  worker_memory    = coalesce(var.worker_memory_mb, var.memory_mb)
  worker_os_disk   = coalesce(var.worker_os_disk_size_gb, var.os_disk_size_gb)
}

# =============================================================================
# BOOTSTRAP NODE
# =============================================================================

resource "proxmox_virtual_environment_vm" "bootstrap" {
  count = var.bootstrap_enabled ? 1 : 0

  name      = "${var.cluster_name}-bootstrap"
  node_name = var.target_node
  vm_id     = var.vmid_base

  on_boot = false

  cpu {
    cores   = local.bootstrap_cpu
    sockets = 1
    type    = var.cpu_type
    numa    = var.numa_enabled
  }

  memory {
    dedicated = local.bootstrap_memory
  }


  # disabled for faster terraform operations
  agent {
    enabled = false
  }

  # faster than waiting for graceful shutdown
  stop_on_destroy = true

  scsi_hardware = "virtio-scsi-single"

  startup {
    order      = 1
    up_delay   = 30
    down_delay = 120
  }

  disk {
    datastore_id = var.os_storage
    size         = var.os_disk_size_gb
    interface    = "scsi0"
    iothread     = true
    ssd          = false
    discard      = "on"
    serial       = "OS-DISK"
  }

  cdrom {
    file_id   = var.bootstrap_iso
    interface = "ide2"
  }

  network_device {
    bridge = var.bridge
    model  = "virtio"
  }

  dynamic "network_device" {
    for_each = var.additional_networks
    content {
      bridge  = network_device.value.bridge
      model   = network_device.value.model
      vlan_id = network_device.value.tag
    }
  }

  tags = var.vm_tags

  description = "OKD Bootstrap Node for cluster ${var.cluster_name}"

  operating_system {
    type = "l26" # linux kernel 2.6+
  }

  machine = "q35"

  bios = "ovmf"

  boot_order = ["scsi0", "ide2", "net0"]

  efi_disk {
    datastore_id = var.os_storage
    type         = "4m"
  }


  lifecycle {
    precondition {
      condition     = var.bootstrap_iso != ""
      error_message = "bootstrap_iso must be provided when bootstrap is enabled."
    }
    ignore_changes = [
      # bpg/terraform-provider-proxmox dynamic + static network_device coexist;
      # the provider produces spurious diffs unless the static block is ignored.
      network_device,
      startup,
      cdrom,
      boot_order,
      # efi_disk holds nvram/boot-order state across reboots; replacing the
      # disk on a force-new attribute change would reset bootloader picks.
      efi_disk,
    ]
  }
}

# =============================================================================
# MASTER NODES
# =============================================================================

resource "proxmox_virtual_environment_vm" "master" {
  count = length(local.masters)

  name      = local.masters[count.index]
  node_name = length(var.master_target_nodes) > count.index ? var.master_target_nodes[count.index] : var.target_node
  vm_id     = var.vmid_base + 10 + count.index

  on_boot = false

  cpu {
    cores   = local.master_cpu
    sockets = 1
    type    = var.cpu_type
    numa    = var.numa_enabled
  }

  memory {
    dedicated = local.master_memory
  }


  # disabled for faster terraform operations
  agent {
    enabled = false
  }

  # faster than waiting for graceful shutdown
  stop_on_destroy = true

  scsi_hardware = "virtio-scsi-single"

  startup {
    order      = 2
    up_delay   = 30
    down_delay = 120
  }

  disk {
    datastore_id = var.os_storage
    size         = local.master_os_disk
    interface    = "scsi0"
    iothread     = true
    ssd          = false
    discard      = "on"
    serial       = "OS-DISK"
  }

  dynamic "disk" {
    for_each = var.master_data_disk_size_gb > 0 ? [1] : []
    content {
      datastore_id = var.data_storage
      size         = var.master_data_disk_size_gb
      interface    = "scsi1"
      iothread     = true
      ssd          = false
      discard      = "on"
      serial       = "CEPH-DATA"
    }
  }

  cdrom {
    file_id   = var.master_isos[count.index]
    interface = "ide2"
  }

  network_device {
    bridge = var.bridge
    model  = "virtio"
  }

  dynamic "network_device" {
    for_each = var.additional_networks
    content {
      bridge  = network_device.value.bridge
      model   = network_device.value.model
      vlan_id = network_device.value.tag
    }
  }

  tags = var.vm_tags

  description = "OKD Master Node ${local.masters[count.index]} for cluster ${var.cluster_name}"

  operating_system {
    type = "l26"
  }

  machine = "q35"

  bios = "ovmf"

  boot_order = ["scsi0", "ide2", "net0"]

  efi_disk {
    datastore_id = var.os_storage
    type         = "4m"
  }


  depends_on = [proxmox_virtual_environment_vm.bootstrap]

  lifecycle {
    precondition {
      condition     = length(var.master_isos) >= var.master_count
      error_message = "master_isos must have at least master_count (${var.master_count}) entries, got ${length(var.master_isos)}."
    }
    ignore_changes = [
      # bpg/terraform-provider-proxmox dynamic + static network_device coexist;
      # the provider produces spurious diffs unless the static block is ignored.
      network_device,
      startup,
      cdrom,
      boot_order,
      # efi_disk holds nvram/boot-order state across reboots; replacing the
      # disk on a force-new attribute change would reset bootloader picks.
      efi_disk,
    ]
  }
}

# =============================================================================
# WORKER NODES
# =============================================================================

resource "proxmox_virtual_environment_vm" "worker" {
  count = length(local.workers)

  name      = local.workers[count.index]
  node_name = length(var.worker_target_nodes) > count.index ? var.worker_target_nodes[count.index] : var.target_node
  vm_id     = var.vmid_base + 100 + count.index

  on_boot = false

  started = var.start_workers_immediately

  cpu {
    cores   = local.worker_cpu
    sockets = 1
    type    = var.cpu_type
    numa    = var.numa_enabled
  }

  memory {
    dedicated = local.worker_memory
  }


  # disabled for faster terraform operations
  agent {
    enabled = false
  }

  # faster than waiting for graceful shutdown
  stop_on_destroy = true

  scsi_hardware = "virtio-scsi-single"

  startup {
    order      = 3
    up_delay   = 30
    down_delay = 120
  }

  disk {
    datastore_id = var.os_storage
    size         = local.worker_os_disk
    interface    = "scsi0"
    iothread     = true
    ssd          = false
    discard      = "on"
    serial       = "OS-DISK"
  }

  dynamic "disk" {
    for_each = var.worker_data_disk_size_gb > 0 ? [1] : []
    content {
      datastore_id = var.data_storage
      size         = var.worker_data_disk_size_gb
      interface    = "scsi1"
      iothread     = true
      ssd          = false
      discard      = "on"
      serial       = "CEPH-DATA"
    }
  }

  cdrom {
    file_id   = var.worker_isos[count.index]
    interface = "ide2"
  }

  network_device {
    bridge = var.bridge
    model  = "virtio"
  }

  dynamic "network_device" {
    for_each = var.additional_networks
    content {
      bridge  = network_device.value.bridge
      model   = network_device.value.model
      vlan_id = network_device.value.tag
    }
  }

  tags = var.vm_tags

  description = "OKD Worker Node ${local.workers[count.index]} for cluster ${var.cluster_name}"

  operating_system {
    type = "l26"
  }

  machine = "q35"

  bios = "ovmf"

  boot_order = ["scsi0", "ide2", "net0"]

  efi_disk {
    datastore_id = var.os_storage
    type         = "4m"
  }


  depends_on = [proxmox_virtual_environment_vm.master]

  lifecycle {
    precondition {
      condition     = length(var.worker_isos) >= var.worker_count
      error_message = "worker_isos must have at least worker_count (${var.worker_count}) entries, got ${length(var.worker_isos)}."
    }
    ignore_changes = [
      # bpg/terraform-provider-proxmox dynamic + static network_device coexist;
      # the provider produces spurious diffs unless the static block is ignored.
      network_device,
      startup,
      cdrom,
      boot_order,
      # efi_disk holds nvram/boot-order state across reboots; replacing the
      # disk on a force-new attribute change would reset bootloader picks.
      efi_disk,
    ]
  }
}
