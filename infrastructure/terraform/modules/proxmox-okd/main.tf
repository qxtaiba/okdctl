# Provider config comes from the parent module; connection env vars and the
# TLS pin are documented in variables.tf.

locals {
  masters = slice(var.master_names, 0, var.master_count)
  workers = slice(var.worker_names, 0, var.worker_count)

  bootstrap_cpu     = coalesce(var.bootstrap_cpu_cores, var.cpu_cores)
  bootstrap_memory  = coalesce(var.bootstrap_memory_mb, var.memory_mb)
  bootstrap_os_disk = var.os_disk_size_gb
  master_cpu        = coalesce(var.master_cpu_cores, var.cpu_cores)
  master_memory     = coalesce(var.master_memory_mb, var.memory_mb)
  master_os_disk    = coalesce(var.master_os_disk_size_gb, var.os_disk_size_gb)
  worker_cpu        = coalesce(var.worker_cpu_cores, var.cpu_cores)
  worker_memory     = coalesce(var.worker_memory_mb, var.memory_mb)
  worker_os_disk    = coalesce(var.worker_os_disk_size_gb, var.os_disk_size_gb)
}

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
    size         = local.bootstrap_os_disk
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
    precondition {
      condition     = local.bootstrap_os_disk >= var.minimum_os_disk_size_gb
      error_message = "os_disk_size_gb (${local.bootstrap_os_disk}) is below minimum_os_disk_size_gb (${var.minimum_os_disk_size_gb}); raise the size or lower the floor deliberately."
    }
    ignore_changes = [
      # bpg/proxmox provider: dynamic + static network_device coexist, causing spurious diffs.
      network_device,
      # HA (ha.tf) and okdctl drive this out-of-band at runtime; create-time value is write-once.
      startup,
      # cdrom.file_id is the per-node ignition ISO; okdctl detaches it after first boot.
      cdrom,
      # boot device set changes once the ignition ISO is detached; don't revert it.
      boot_order,
      # datastore_id stays tracked (an os_storage move relocates it); type reset
      # would lose nvram/boot picks.
      efi_disk[0].type,
    ]
  }
}

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

  # Ceph OSD data disk (not in ignore_changes); only grow
  # master_data_disk_size_gb — shrinking destroys the OSD data.
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

  # Dedicated ceph mon-store disk (mounted at /var/lib/rook via machineconfig);
  # only grow master_mon_disk_size_gb — shrinking destroys it.
  dynamic "disk" {
    for_each = var.master_mon_disk_size_gb > 0 ? [1] : []
    content {
      datastore_id = var.data_storage
      size         = var.master_mon_disk_size_gb
      interface    = "scsi2"
      iothread     = true
      ssd          = false
      discard      = "on"
      serial       = "MON-DATA"
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

  # prevent_destroy must be a literal bool (hashicorp/terraform#3116); a
  # confirmed `okdctl destroy` overrides it via a transient
  # prevent_destroy_override.tf. For a manual destroy, add the same override
  # in this module directory, then destroy and delete the file:
  #   resource "proxmox_virtual_environment_vm" "master" {
  #     lifecycle { prevent_destroy = false }
  #   }
  lifecycle {
    prevent_destroy = true
    precondition {
      condition     = length(var.master_isos) >= var.master_count && alltrue([for iso in slice(var.master_isos, 0, var.master_count) : iso != ""])
      error_message = "master_isos must have at least master_count (${var.master_count}) non-empty entries; got ${length(var.master_isos)} entries (check for empty strings in the first ${var.master_count})."
    }
    precondition {
      condition     = var.master_data_disk_size_gb == 0 || var.master_data_disk_size_gb >= var.minimum_data_disk_size_gb
      error_message = "master_data_disk_size_gb (${var.master_data_disk_size_gb}) is below minimum_data_disk_size_gb (${var.minimum_data_disk_size_gb}); shrinking a data disk destroys its ceph osd data — raise the size or lower the floor deliberately."
    }
    precondition {
      condition     = var.master_mon_disk_size_gb == 0 || var.master_mon_disk_size_gb >= var.minimum_data_disk_size_gb
      error_message = "master_mon_disk_size_gb (${var.master_mon_disk_size_gb}) is below minimum_data_disk_size_gb (${var.minimum_data_disk_size_gb}); a below-floor value must fail the plan, not silently omit (destroy) the mon disk — raise the size or lower the floor deliberately."
    }
    precondition {
      condition     = local.master_os_disk >= var.minimum_os_disk_size_gb
      error_message = "master os disk size (${local.master_os_disk}) is below minimum_os_disk_size_gb (${var.minimum_os_disk_size_gb}); master os disks hold /var/lib/etcd — raise the size or lower the floor deliberately."
    }
    # Same ignore set/reasons as the bootstrap resource above.
    ignore_changes = [
      network_device,
      startup,
      cdrom,
      boot_order,
      efi_disk[0].type,
    ]
  }
}

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

  # Ceph OSD data disk (see master block); only grow worker_data_disk_size_gb
  # — shrinking destroys it.
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

  # No prevent_destroy (hashicorp/terraform#3116 — can't be variable-gated).
  # Removing a worker destroys its ceph data disk — drain first.
  lifecycle {
    precondition {
      condition     = length(var.worker_isos) >= var.worker_count && alltrue([for iso in slice(var.worker_isos, 0, var.worker_count) : iso != ""])
      error_message = "worker_isos must have at least worker_count (${var.worker_count}) non-empty entries; got ${length(var.worker_isos)} entries (check for empty strings in the first ${var.worker_count})."
    }
    precondition {
      condition     = var.worker_data_disk_size_gb == 0 || var.worker_data_disk_size_gb >= var.minimum_data_disk_size_gb
      error_message = "worker_data_disk_size_gb (${var.worker_data_disk_size_gb}) is below minimum_data_disk_size_gb (${var.minimum_data_disk_size_gb}); shrinking a data disk destroys its ceph osd data — raise the size or lower the floor deliberately."
    }
    precondition {
      condition     = local.worker_os_disk >= var.minimum_os_disk_size_gb
      error_message = "worker os disk size (${local.worker_os_disk}) is below minimum_os_disk_size_gb (${var.minimum_os_disk_size_gb}); raise the size or lower the floor deliberately."
    }
    # Same ignore set/reasons as the bootstrap resource above.
    ignore_changes = [
      network_device,
      startup,
      cdrom,
      boot_order,
      efi_disk[0].type,
    ]
  }
}
