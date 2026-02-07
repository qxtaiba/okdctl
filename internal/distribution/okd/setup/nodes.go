package setup

import (
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
)

// BuildNodeList creates a list of all cluster nodes.
func (p *Phase) BuildNodeList(cfg *config.Config) ([]NodeInfo, error) {
	var nodes []NodeInfo

	startIP := cfg.Networking.StaticIP.Start
	baseIP, lastOctet, err := netutil.SplitIPv4(startIP)
	if err != nil {
		return nil, utils.WrapError("invalid start IP", err)
	}

	bootstrapIP := fmt.Sprintf("%s.%d", baseIP, lastOctet)
	nodes = append(nodes, NodeInfo{
		Name: "bootstrap",
		Role: "bootstrap",
		IP:   bootstrapIP,
	})

	for i := 0; i < cfg.Topology.ControlPlane.Count; i++ {
		newOctet := lastOctet + 1 + i
		if newOctet > 255 {
			return nil, fmt.Errorf("IP address range exceeded: cannot assign IP for master%d", i)
		}
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
		if newOctet > 255 {
			return nil, fmt.Errorf("IP address range exceeded: cannot assign IP for worker%d", i)
		}
		ip := fmt.Sprintf("%s.%d", baseIP, newOctet)
		nodes = append(nodes, NodeInfo{
			Name: fmt.Sprintf("worker%d", i),
			Role: "worker",
			IP:   ip,
		})
	}

	return nodes, nil
}
