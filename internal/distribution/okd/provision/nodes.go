package provision

import (
	"fmt"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// BuildNodeList returns the ordered list of nodes (bootstrap, masters,
// workers) with IPs allocated from the static-IP start. The IP range is
// validated against machineCIDR up front so we fail before per-node
// calculation.
func BuildNodeList(cfg *config.Config) ([]NodeInfo, error) {
	var nodes []NodeInfo

	startIP := cfg.Networking.StaticIP.Start

	totalNodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
	if cfg.Networking.MachineCIDR != "" {
		if err := netutil.ValidateIPRangeInCIDR(startIP, totalNodes, cfg.Networking.MachineCIDR); err != nil {
			return nil, &errtypes.ConfigError{Msg: "static IP range does not fit in machine CIDR", Err: err}
		}
	}

	nodes = append(nodes, NodeInfo{
		Name: string(nodetypes.RoleBootstrap),
		Role: nodetypes.RoleBootstrap,
		IP:   startIP,
	})

	for i := range cfg.Topology.ControlPlane.Count {
		ip, err := netutil.CalculateVMIP(startIP, 1+i)
		if err != nil {
			return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("calculate %s%d IP", nodetypes.RoleMaster, i), Err: err}
		}
		nodes = append(nodes, NodeInfo{
			Name: fmt.Sprintf("%s%d", nodetypes.RoleMaster, i),
			Role: nodetypes.RoleMaster,
			IP:   ip,
		})
	}

	workerOffset := 1 + cfg.Topology.ControlPlane.Count
	for i := range cfg.Topology.Workers.Count {
		ip, err := netutil.CalculateVMIP(startIP, workerOffset+i)
		if err != nil {
			return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("calculate %s%d IP", nodetypes.RoleWorker, i), Err: err}
		}
		nodes = append(nodes, NodeInfo{
			Name: fmt.Sprintf("%s%d", nodetypes.RoleWorker, i),
			Role: nodetypes.RoleWorker,
			IP:   ip,
		})
	}

	return nodes, nil
}
