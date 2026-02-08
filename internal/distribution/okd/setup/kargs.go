package setup

import (
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
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

// BuildDestKargs returns networking kernel arguments to persist into the installed OS.
// Unlike BuildLiveKargs these omit coreos.inst.* directives which are only
// relevant during the live installer session.
func BuildDestKargs(params LiveKargsParams) []string {
	return []string{
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
	if dns == "" {
		dns = cfg.Networking.Bastion.IP
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
