package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	minVMIDBase      = 100
	maxVLAN          = 4094
	minCPUModelLevel = 3
	cpuTypeHost      = "host"
	cpuModelPrefix   = "x86-64-v"
)

func (c *Config) validateProvider(r *ValidationResult, nodes []nodeRef) {
	if c.Provider.Type != ProviderProxmox {
		r.addf(FieldProviderType, "must be %s", ProviderProxmox)
	}
	px := &c.Provider.Proxmox
	validateEndpoint(r, px.Endpoint)
	if len(px.Nodes) == 0 {
		r.AddError(FieldProxmoxNodes, "list at least one Proxmox node")
	}
	for i, node := range px.Nodes {
		if node == "" {
			r.AddError(fmt.Sprintf("%s[%d]", FieldProxmoxNodes, i), "required")
		}
	}
	for _, f := range []struct{ field, value string }{
		{FieldProxmoxStorage, px.Storage},
		{FieldProxmoxISOStorage, px.ISOStorage},
		{FieldProxmoxBridge, px.Bridge},
	} {
		if f.value == "" {
			r.AddError(f.field, "required")
		}
	}
	if px.VLAN < 0 || px.VLAN > maxVLAN {
		r.addf(FieldProxmoxVLAN, "%d must be 0 (untagged) or 1..%d", px.VLAN, maxVLAN)
	}
	if !cpuTypeSupported(px.CPUType) {
		r.addf(FieldProxmoxCPUType, "%q must be %s or %s%d or newer", px.CPUType, cpuTypeHost, cpuModelPrefix, minCPUModelLevel)
	}
	if px.VMIDBase < minVMIDBase {
		r.addf(FieldProxmoxVMIDBase, "%d must be at least %d", px.VMIDBase, minVMIDBase)
	}
	// 24 bits also keeps the okd<vmid>d<n> disk serial within Proxmox's 20 characters.
	if n := len(nodes); n > 0 && nodes[n-1].vmid > MaxVMID {
		r.addf(FieldProxmoxVMIDBase, "highest vmid %d exceeds %d (24 bits)", nodes[n-1].vmid, MaxVMID)
	}
}

func validateEndpoint(r *ValidationResult, endpoint string) {
	if endpoint == "" {
		r.AddError(FieldProxmoxEndpoint, "required")
		return
	}
	u, err := url.Parse(endpoint)
	switch {
	case err != nil || u.Scheme == "" || u.Host == "":
		r.addf(FieldProxmoxEndpoint, "%q must be a URL with scheme and host, such as https://pve:8006", endpoint)
	case u.Scheme != "http" && u.Scheme != "https":
		r.addf(FieldProxmoxEndpoint, "%q must use http or https", endpoint)
	}
}

// cpuTypeSupported accepts host or an x86-64-v<N> model at or above v3, the
// level the agent ISO's RHCOS requires.
func cpuTypeSupported(cpuType string) bool {
	if cpuType == cpuTypeHost {
		return true
	}
	level, ok := strings.CutPrefix(cpuType, cpuModelPrefix)
	if !ok {
		return false
	}
	n, err := strconv.Atoi(level)
	return err == nil && n >= minCPUModelLevel
}
