package nodetypes

import (
	"fmt"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/netutil"
)

// ClusterNode is one VM in the cluster topology: role, per-role index, and the
// static IP allocated from the configured start address.
type ClusterNode struct {
	Role  NodeRole
	Index int
	IP    string
}

// Name returns the role-scoped node name ("bootstrap", "master0", "worker1")
// used for ISO filenames and DNS/HAProxy backend entries.
func (n ClusterNode) Name() string {
	if n.Role == RoleBootstrap {
		return string(n.Role)
	}
	return fmt.Sprintf("%s%d", n.Role, n.Index)
}

// PrefixedName returns the cluster-scoped VM name ("<cluster>-master0"), the
// single encoding shared by Terraform VM names and DNS host entries.
func (n ClusterNode) PrefixedName(clusterName string) string {
	return clusterName + "-" + n.Name()
}

// ClusterNodes enumerates cfg's topology in provisioning order (bootstrap,
// masters, workers) with IPs offset sequentially from the static-IP start. The
// machine CIDR, when configured, is validated up front so callers fail before
// any per-node calculation.
func ClusterNodes(cfg *config.Config) ([]ClusterNode, error) {
	startIP := cfg.Networking.StaticIP.Start

	// Bounded here rather than left to validateResources: the deploy gate runs
	// a narrower scope, so a hand-edited count can reach this sum unchecked and
	// overflow it into a negative make capacity.
	masters, workers := cfg.Topology.ControlPlane.Count, cfg.Topology.Workers.Count
	if masters < 0 || masters > config.MaxNodeCount || workers < 0 || workers > config.MaxNodeCount {
		return nil, (&errtypes.ConfigError{
			Msg: fmt.Sprintf("node count out of range: %d control plane, %d workers", masters, workers),
		}).WithHint(fmt.Sprintf("set each count between 0 and %d", config.MaxNodeCount))
	}

	total := 1 + masters + workers
	if cfg.Networking.MachineCIDR != "" {
		if err := netutil.ValidateIPRangeInCIDR(startIP, total, cfg.Networking.MachineCIDR); err != nil {
			return nil, &errtypes.ConfigError{Msg: "static IP range does not fit in machine CIDR", Err: err}
		}
	}

	nodes := make([]ClusterNode, 0, total)
	nodes = append(nodes, ClusterNode{Role: RoleBootstrap, IP: startIP})

	for i := range masters {
		ip, err := netutil.CalculateVMIP(startIP, 1+i)
		if err != nil {
			return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("calculate %s%d IP", RoleMaster, i), Err: err}
		}
		nodes = append(nodes, ClusterNode{Role: RoleMaster, Index: i, IP: ip})
	}

	workerOffset := 1 + masters
	for i := range workers {
		ip, err := netutil.CalculateVMIP(startIP, workerOffset+i)
		if err != nil {
			return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("calculate %s%d IP", RoleWorker, i), Err: err}
		}
		nodes = append(nodes, ClusterNode{Role: RoleWorker, Index: i, IP: ip})
	}

	return nodes, nil
}
