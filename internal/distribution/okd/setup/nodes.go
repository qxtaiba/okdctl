package setup

import (
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
)

func (p *Phase) BuildNodeList(cfg *config.Config) ([]NodeInfo, error) {
	var nodes []NodeInfo

	startIP := cfg.Networking.StaticIP.Start
	baseIP, lastOctet, err := netutil.SplitIPv4(startIP)
	if err != nil {
		return nil, utils.WrapError("invalid start IP", err)
	}

	if lastOctet < 0 || lastOctet > 255 {
		return nil, fmt.Errorf("start IP %q has invalid last octet %d", startIP, lastOctet)
	}

	totalNodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
	highest := lastOctet + totalNodes - 1
	if highest > 255 {
		return nil, fmt.Errorf("IP range insufficient: start %q + %d nodes overflows subnet", startIP, totalNodes)
	}

	bootstrapIP := fmt.Sprintf("%s.%d", baseIP, lastOctet)
	nodes = append(nodes, NodeInfo{
		Name: "bootstrap",
		Role: "bootstrap",
		IP:   bootstrapIP,
	})

	for i := 0; i < cfg.Topology.ControlPlane.Count; i++ {
		newOctet := lastOctet + 1 + i
		ip := fmt.Sprintf("%s.%d", baseIP, newOctet)
		nodes = append(nodes, NodeInfo{
			Name: fmt.Sprintf("master%d", i),
			Role: "master",
			IP:   ip,
		})
	}

	workerOffset := 1 + cfg.Topology.ControlPlane.Count
	for i := 0; i < cfg.Topology.Workers.Count; i++ {
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
