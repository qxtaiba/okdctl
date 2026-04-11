package setup

import (
	"fmt"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/utils/netutil"
)

func (p *Phase) BuildNodeList(cfg *config.Config) ([]NodeInfo, error) {
	var nodes []NodeInfo

	startIP := cfg.Networking.StaticIP.Start
	baseIP, lastOctet, err := netutil.SplitIPv4(startIP)
	if err != nil {
		return nil, fmt.Errorf("invalid start IP: %w", err)
	}

	totalNodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
	if err := netutil.ValidateIPRangeInCIDR(startIP, totalNodes, cfg.Networking.MachineCIDR); err != nil {
		return nil, err
	}

	bootstrapIP := fmt.Sprintf("%s.%d", baseIP, lastOctet)
	nodes = append(nodes, NodeInfo{
		Name: "bootstrap",
		Role: "bootstrap",
		IP:   bootstrapIP,
	})

	for i := range cfg.Topology.ControlPlane.Count {
		newOctet := lastOctet + 1 + i
		ip := fmt.Sprintf("%s.%d", baseIP, newOctet)
		nodes = append(nodes, NodeInfo{
			Name: fmt.Sprintf("master%d", i),
			Role: "master",
			IP:   ip,
		})
	}

	workerOffset := 1 + cfg.Topology.ControlPlane.Count
	for i := range cfg.Topology.Workers.Count {
		newOctet := lastOctet + workerOffset + i
		ip := fmt.Sprintf("%s.%d", baseIP, newOctet)
		nodes = append(nodes, NodeInfo{
			Name: fmt.Sprintf("worker%d", i),
			Role: "worker",
			IP:   ip,
		})
	}

	return nodes, nil
}
