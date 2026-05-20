package setup

import (
	"fmt"
	"net/netip"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
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
// FCOS install (ignition URL plus static networking). The CA cert that
// authenticates the HTTPS ignition server is embedded into the live ISO via
// `coreos-installer iso customize --ignition-ca <file>` — not as a karg.
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
// from cfg, applying netutil defaults for netmask/interface and the
// bastion IP for DNS when unset.
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
		iface = netutil.DefaultProxmoxIface
	}

	return gateway, netmask, dns, iface
}

// BuildIgnitionURLForNode builds the https:// ignition URL a node of the
// given role fetches during FCOS first-boot. The payload contains the
// cluster pull-secret, SSH authorized keys, and machine-config tokens;
// TLS with a pinned CA cert is the primary defence against credential
// capture over the machine-network VLAN. IgnitionServerIP must be RFC1918,
// loopback, or link-local to prevent exposure on public interfaces.
func BuildIgnitionURLForNode(cfg *config.Config, role phase.NodeRole) (string, error) {
	ignitionIP := cfg.HTTPServer.IgnitionServerIP

	addr, err := netip.ParseAddr(ignitionIP)
	if err != nil {
		return "", &errtypes.ConfigError{Msg: fmt.Sprintf("ignition server IP %q is not a valid IP address", ignitionIP), Err: err}
	}
	if !addr.IsPrivate() && !addr.IsLoopback() && !addr.IsLinkLocalUnicast() {
		return "", &errtypes.ConfigError{Msg: fmt.Sprintf("ignition server IP %q must be RFC1918, loopback, or link-local — HTTPS ignition on a public address exposes cluster credentials", ignitionIP)}
	}

	ignitionPort := cfg.HTTPServer.Port
	if ignitionPort == 0 {
		ignitionPort = DefaultIgnitionHTTPSPort
	}
	ignitionFile := role.String() + ".ign"
	if ignitionPort == DefaultIgnitionHTTPSPort {
		return fmt.Sprintf("https://%s/ignition/%s", ignitionIP, ignitionFile), nil
	}
	return fmt.Sprintf("https://%s:%d/ignition/%s", ignitionIP, ignitionPort, ignitionFile), nil
}
