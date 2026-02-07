#!/bin/bash
set -euo pipefail

# Disk serials assigned in terraform (proxmox-okd/main.tf).
OS_SERIAL="OS-DISK"
DATA_SERIAL="CEPH-DATA"
OS_DISK=""
DATA_DISK=""

# discover disks by serial number
for dev in /dev/sd?; do
  [ -b "$dev" ] || continue
  serial=$(lsblk -ndo SERIAL "$dev" 2>/dev/null) || continue
  case "$serial" in
    "$OS_SERIAL")   OS_DISK="$dev" ;;
    "$DATA_SERIAL") DATA_DISK="$dev" ;;
  esac
done

# fallback: if only one disk found by serial, the other is the remaining /dev/sd?
all_disks=( /dev/sd? )
if [ -z "$OS_DISK" ] && [ -n "$DATA_DISK" ] && [ "${#all_disks[@]}" -eq 2 ]; then
  for dev in "${all_disks[@]}"; do
    [ "$dev" != "$DATA_DISK" ] && OS_DISK="$dev"
  done
fi
if [ -z "$DATA_DISK" ] && [ -n "$OS_DISK" ] && [ "${#all_disks[@]}" -eq 2 ]; then
  for dev in "${all_disks[@]}"; do
    [ "$dev" != "$OS_DISK" ] && DATA_DISK="$dev"
  done
fi

# last resort: single-disk VM, use it as OS disk
if [ -z "$OS_DISK" ] && [ -z "$DATA_DISK" ] && [ "${#all_disks[@]}" -eq 1 ]; then
  OS_DISK="${all_disks[0]}"
fi

# write dest-device for coreos-installer
if [ -n "$OS_DISK" ]; then
  mkdir -p /etc/coreos/installer.d
  echo "--dest-device ${OS_DISK}" > /etc/coreos/installer.d/50-dest-device.conf
fi

# wipe data disk so stale partition labels don't confuse the installed system
if [ -n "$DATA_DISK" ]; then
  sgdisk --zap-all "$DATA_DISK"
  wipefs --all "$DATA_DISK"
fi
