package cli

import (
	"cmp"
	"slices"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/proxmox"
)

// preflightInput maps the raw v2 provider block to a footprint with no node identities.
func preflightInput(cfg *config.Config) proxmox.PreflightInput {
	px := cfg.Provider.Proxmox
	if px == nil {
		return proxmox.PreflightInput{}
	}
	var nodes []string
	for _, n := range slices.Concat([]string{px.Node}, px.ControlPlaneNodes, px.WorkerNodes) {
		if n != "" && !slices.Contains(nodes, n) {
			nodes = append(nodes, n)
		}
	}
	return proxmox.PreflightInput{
		Nodes:      nodes,
		Storage:    px.Storage,
		ISOStorage: cmp.Or(px.ISOStorage, px.Storage),
		Bridge:     cmp.Or(px.Bridge, "vmbr0"),
		HA:         px.HAEnabled,
		CPUModel:   px.CPUType,
	}
}
