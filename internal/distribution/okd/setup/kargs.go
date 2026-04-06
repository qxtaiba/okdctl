package setup

import (
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
)

type LiveKargsParams struct {
	NodeIP      string
	Gateway     string
	Netmask     string
	DNS         string
	Interface   string
	IgnitionURL string
}

func BuildLiveKargs(params LiveKargsParams) []string {
	return []string{
		fmt.Sprintf("coreos.inst.ignition_url=%s", params.IgnitionURL),
		fmt.Sprintf("ip=%s::%s:%s::%s:none", params.NodeIP, params.Gateway, params.Netmask, params.Interface),
		fmt.Sprintf("nameserver=%s", params.DNS),
	}
}

// BuildDestKargs returns networking kernel arguments persisted into the installed OS,
// omitting coreos.inst.* directives (only relevant during the live installer session).
//
// Repeated ISO builds using these kargs are idempotent: each invocation of
// buildNodeISO overwrites the target .iso file at outputPath before passing
// --dest-karg-append to coreos-installer, so kargs are never appended onto a
// pre-existing ISO from a prior run. Regenerating an ISO with the same
// parameters produces the same result; changing network fields (e.g. IP,
// DNS) simply replaces the previous ISO wholesale.
func BuildDestKargs(params LiveKargsParams) []string {
	return []string{
		fmt.Sprintf("ip=%s::%s:%s::%s:none", params.NodeIP, params.Gateway, params.Netmask, params.Interface),
		fmt.Sprintf("nameserver=%s", params.DNS),
	}
}

// ExtractNetworkConfig returns network params from config, using top-level gateway directly.
func ExtractNetworkConfig(cfg *config.Config) (gateway, netmask, dns, iface string) {
	staticCfg := cfg.Networking.StaticIP

	gateway = cfg.Networking.Gateway

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

func BuildIgnitionURLForNode(cfg *config.Config, role string) string {
	ignitionIP := cfg.HTTPServer.IgnitionServerIP
	ignitionPort := cfg.HTTPServer.Port
	if ignitionPort == 0 {
		ignitionPort = 8080
	}
	ignitionFile := role + ".ign"
	return fmt.Sprintf("http://%s:%d/ignition/%s", ignitionIP, ignitionPort, ignitionFile)
}
