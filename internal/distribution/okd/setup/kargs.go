package setup

import (
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
)

const (
	// OSDiskSerial is the disk serial assigned to the OS disk in terraform.
	OSDiskSerial = "OS-DISK"

	// DataDiskSerial is the disk serial assigned to the data disk in terraform.
	DataDiskSerial = "CEPH-DATA"
)

// LiveKargsParams holds parameters for building live-session kernel arguments.
type LiveKargsParams struct {
	NodeIP      string
	Gateway     string
	Netmask     string
	DNS         string
	Interface   string
	IgnitionURL string
}

// BuildLiveKargs constructs the live-session kernel arguments for ISO customization.
func BuildLiveKargs(params LiveKargsParams) []string {
	return []string{
		fmt.Sprintf("coreos.inst.ignition_url=%s", params.IgnitionURL),
		fmt.Sprintf("ip=%s::%s:%s::%s:none", params.NodeIP, params.Gateway, params.Netmask, params.Interface),
		fmt.Sprintf("nameserver=%s", params.DNS),
	}
}

// BuildWorkerPreInstallScript returns a shell script that runs before coreos-installer
// on worker nodes. It discovers the OS disk and data disk by serial number using lsblk,
// writes the OS disk as --dest-device to /etc/coreos/installer.d/, and wipes the data disk.
func BuildWorkerPreInstallScript() string {
	return fmt.Sprintf(`#!/bin/bash
set -euo pipefail

OS_SERIAL="%s"
DATA_SERIAL="%s"
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
`, OSDiskSerial, DataDiskSerial)
}

// ExtractNetworkConfig extracts network configuration from config.
// Each field prefers the StaticIP-specific value, falling back to the top-level or a default.
func ExtractNetworkConfig(cfg *config.Config) (gateway, netmask, dns, iface string) {
	staticCfg := cfg.Networking.StaticIP

	gateway = staticCfg.Gateway
	if gateway == "" {
		gateway = cfg.Networking.Gateway
	}

	netmask = staticCfg.Netmask
	if netmask == "" {
		netmask = "255.255.255.0"
	}

	dns = staticCfg.DNS
	if dns == "" && len(cfg.Networking.DNS) > 0 {
		dns = cfg.Networking.DNS[0]
	}

	iface = staticCfg.Interface
	if iface == "" {
		iface = "ens18"
	}

	return gateway, netmask, dns, iface
}

// BuildIgnitionURLForNode constructs the full ignition file URL for a specific node role.
func BuildIgnitionURLForNode(cfg *config.Config, role string) string {
	ignitionIP := cfg.HTTPServer.IgnitionServerIP
	ignitionPort := cfg.HTTPServer.Port
	if ignitionPort == 0 {
		ignitionPort = 8080
	}
	ignitionFile := role + ".ign"
	return fmt.Sprintf("http://%s:%d/ignition/%s", ignitionIP, ignitionPort, ignitionFile)
}
