#!/bin/bash
set -euo pipefail

# Disk serials assigned in terraform (proxmox-okd/main.tf).
# If either serial is missing the VM is misconfigured — fail loudly
# so coreos-installer aborts rather than silently wiping the wrong disk.
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

if [ -z "$OS_DISK" ]; then
  echo "FATAL: no disk with serial '$OS_SERIAL' found" >&2
  exit 1
fi

# write dest-device for coreos-installer
mkdir -p /etc/coreos/installer.d
echo "--dest-device ${OS_DISK}" > /etc/coreos/installer.d/50-dest-device.conf

# wipe data disk so stale partition labels don't confuse the installed system
if [ -n "$DATA_DISK" ]; then
  sgdisk --zap-all "$DATA_DISK"
  wipefs --all "$DATA_DISK"
else
  echo "WARNING: no disk with serial '$DATA_SERIAL' found, skipping wipe" >&2
fi
