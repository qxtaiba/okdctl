package setup

import (
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
)

// KernelArgsParams holds parameters for building CoreOS kernel arguments.
type KernelArgsParams struct {
	NodeIP      string
	Gateway     string
	Netmask     string
	DNS         string
	Interface   string
	IgnitionURL string
	InstallDev  string
}

// BuildKernelArgs constructs the kernel arguments for CoreOS installation.
func BuildKernelArgs(params KernelArgsParams) []string {
	installDev := params.InstallDev
	if installDev == "" {
		installDev = "/dev/sda"
	}

	return []string{
		fmt.Sprintf("coreos.inst.install_dev=%s", installDev),
		fmt.Sprintf("coreos.inst.ignition_url=%s", params.IgnitionURL),
		fmt.Sprintf("ip=%s::%s:%s::%s:none", params.NodeIP, params.Gateway, params.Netmask, params.Interface),
		fmt.Sprintf("nameserver=%s", params.DNS),
	}
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
