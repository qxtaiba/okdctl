package proxmox

import (
	"cmp"
	"net/netip"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/model"
)

// PreflightInputFor derives the cluster's footprint on the Proxmox hosts from
// the node inventory and the provider block. An empty tag means no cluster
// record exists yet, so the preview MACs are left out of the collision check;
// isoBytes is 0 until the agent ISO is built.
func PreflightInputFor(inv *model.Inventory, px *config.ProxmoxConfig, tag string, isoBytes int64) PreflightInput {
	in := PreflightInput{
		Nodes:           px.Nodes,
		Storage:         px.Storage,
		ISOStorage:      cmp.Or(px.ISOStorage, px.Storage),
		Bridge:          px.Bridge,
		VLAN:            px.VLAN,
		Pool:            px.Pool,
		HA:              px.HA,
		Tag:             tag,
		VMIDs:           make([]int, 0, len(inv.Nodes)),
		NodeAddresses:   make(map[netip.Addr]int, len(inv.Nodes)),
		VIPs:            []netip.Addr{inv.APIVIP, inv.IngressVIP},
		MemoryMiBByNode: make(map[string]int, len(px.Nodes)),
		ISOBytes:        isoBytes,
	}
	if tag != "" {
		in.MACs = make([]string, 0, len(inv.Nodes))
	}
	for i := range inv.Nodes {
		n := &inv.Nodes[i]
		in.VMIDs = append(in.VMIDs, n.VMID)
		in.NodeAddresses[n.IP] = n.VMID
		in.MemoryMiBByNode[n.Host] += n.MemoryMiB
		if tag != "" {
			in.MACs = append(in.MACs, n.MAC)
		}
	}
	return in
}
