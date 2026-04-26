package setup

import (
	"fmt"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/netutil"
)

// LiveKargsParams carries the per-node values embedded as kernel arguments
// in the custom live ISO and on the installed system.
type LiveKargsParams struct {
	NodeIP      string
	Gateway     string
	Netmask     string
	DNS         string
	Interface   string
	IgnitionURL string
}

// BuildLiveKargs returns the kernel arguments used by the live ISO during
// FCOS install (ignition URL plus static networking).
func BuildLiveKargs(params *LiveKargsParams) []string {
	return []string{
		fmt.Sprintf("coreos.inst.ignition_url=%s", params.IgnitionURL),
		fmt.Sprintf("ip=%s::%s:%s::%s:none", params.NodeIP, params.Gateway, params.Netmask, params.Interface),
		fmt.Sprintf("nameserver=%s", params.DNS),
	}
}

// BuildDestKargs returns persistent networking kernel arguments for the
// installed system. Idempotent across rebuilds (each buildNodeISO
// overwrites its output).
func BuildDestKargs(params *LiveKargsParams) []string {
	return []string{
		fmt.Sprintf("ip=%s::%s:%s::%s:none", params.NodeIP, params.Gateway, params.Netmask, params.Interface),
		fmt.Sprintf("nameserver=%s", params.DNS),
	}
}

// ExtractNetworkConfig returns the networking kargs seed values derived
// from cfg, applying the 255.255.255.0 / bastion-IP / "ens18" defaults
// when unset.
func ExtractNetworkConfig(cfg *config.Config) (gateway, netmask, dns, iface string) {
	staticCfg := cfg.Networking.StaticIP

	gateway = cfg.Networking.Gateway

	netmask = staticCfg.Netmask
	if netmask == "" {
		netmask = netutil.DefaultNetmask
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

// BuildIgnitionURLForNode builds the http:// ignition URL a node of the
// given role fetches during FCOS first-boot.
func BuildIgnitionURLForNode(cfg *config.Config, role phase.NodeRole) string {
	ignitionIP := cfg.HTTPServer.IgnitionServerIP
	ignitionPort := cfg.HTTPServer.Port
	if ignitionPort == 0 {
		ignitionPort = DefaultIgnitionPort
	}
	ignitionFile := role.String() + ".ign"
	return fmt.Sprintf("http://%s:%d/ignition/%s", ignitionIP, ignitionPort, ignitionFile)
}
