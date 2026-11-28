package cli

import (
	"cmp"
	"net/netip"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/model"
	"github.com/qxtaiba/okdctl/internal/proxmox"
)

// preflightInput derives the cluster's Proxmox footprint from cfg through the
// node model; the VM tag and MACs come only from a written cluster record,
// since preview identities are not the cluster's.
func preflightInput(cfg *config.Config, configPath string) (proxmox.PreflightInput, error) {
	infraID, recorded, err := recordedInfraID(configPath, cfg.Cluster.Name)
	if err != nil {
		return proxmox.PreflightInput{}, err
	}
	inv, err := model.Build(cfg, infraID)
	if err != nil {
		return proxmox.PreflightInput{}, &errtypes.ConfigError{Msg: "derive node inventory: " + err.Error(), Err: err}
	}
	px := &cfg.Provider.Proxmox
	// ISOBytes stays 0: the install builds the agent ISO after doctor runs.
	in := proxmox.PreflightInput{
		Nodes:           px.Nodes,
		Storage:         px.Storage,
		ISOStorage:      cmp.Or(px.ISOStorage, px.Storage),
		Bridge:          px.Bridge,
		VLAN:            px.VLAN,
		Pool:            px.Pool,
		HA:              px.HA,
		CPUModel:        px.CPUType,
		VMIDs:           make([]int, 0, len(inv.Nodes)),
		NodeAddresses:   make(map[netip.Addr]int, len(inv.Nodes)),
		VIPs:            []netip.Addr{inv.APIVIP, inv.IngressVIP},
		MemoryMiBByNode: make(map[string]int, len(px.Nodes)),
	}
	if recorded {
		in.Tag = infraID
		in.MACs = make([]string, 0, len(inv.Nodes))
	}
	for i := range inv.Nodes {
		n := &inv.Nodes[i]
		in.VMIDs = append(in.VMIDs, n.VMID)
		in.NodeAddresses[n.IP] = n.VMID
		in.MemoryMiBByNode[n.Host] += n.MemoryMiB
		if recorded {
			in.MACs = append(in.MACs, n.MAC)
		}
	}
	return in, nil
}
