package config

import "fmt"

func (c *Config) validateProvider(r *ValidationResult, nodes []nodeRef) {
	if c.Provider.Type != ProviderProxmox {
		r.addf(FieldProviderType, "must be %s", ProviderProxmox)
	}
	px := &c.Provider.Proxmox
	r.check(FieldProxmoxEndpoint, CheckEndpoint(px.Endpoint))
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
	if px.VLAN < 0 || px.VLAN > MaxVLAN {
		r.addf(FieldProxmoxVLAN, "%d must be 0 (untagged) or 1..%d", px.VLAN, MaxVLAN)
	}
	r.check(FieldProxmoxCPUType, CheckCPUType(px.CPUType))
	if px.VMIDBase < MinVMIDBase {
		r.addf(FieldProxmoxVMIDBase, "%d must be at least %d", px.VMIDBase, MinVMIDBase)
	}
	// 24 bits also keeps the okd<vmid>d<n> disk serial within Proxmox's 20 characters.
	if n := len(nodes); n > 0 && nodes[n-1].vmid > MaxVMID {
		r.addf(FieldProxmoxVMIDBase, "highest vmid %d exceeds %d (24 bits)", nodes[n-1].vmid, MaxVMID)
	}
}
