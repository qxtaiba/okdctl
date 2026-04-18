package setup

import (
	"fmt"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/netutil"
)

func (p *Phase) BuildNodeList(cfg *config.Config) ([]NodeInfo, error) {
	var nodes []NodeInfo

	startIP := cfg.Networking.StaticIP.Start

	totalNodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
	if cfg.Networking.MachineCIDR != "" {
		if err := netutil.ValidateIPRangeInCIDR(startIP, totalNodes, cfg.Networking.MachineCIDR); err != nil {
			return nil, err
		}
	}

	nodes = append(nodes, NodeInfo{
		Name: "bootstrap",
		Role: phase.RoleBootstrap,
		IP:   startIP,
	})

	for i := range cfg.Topology.ControlPlane.Count {
		ip, err := netutil.CalculateVMIP(startIP, 1+i)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate master%d IP: %w", i, err)
		}
		nodes = append(nodes, NodeInfo{
			Name: fmt.Sprintf("master%d", i),
			Role: phase.RoleMaster,
			IP:   ip,
		})
	}

	workerOffset := 1 + cfg.Topology.ControlPlane.Count
	for i := range cfg.Topology.Workers.Count {
		ip, err := netutil.CalculateVMIP(startIP, workerOffset+i)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate worker%d IP: %w", i, err)
		}
		nodes = append(nodes, NodeInfo{
			Name: fmt.Sprintf("worker%d", i),
			Role: phase.RoleWorker,
			IP:   ip,
		})
	}

	return nodes, nil
}
