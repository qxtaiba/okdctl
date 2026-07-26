package provision

import (
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// BuildNodeList returns the ordered list of nodes (bootstrap, masters,
// workers) with IPs allocated from the static-IP start, projected from
// nodetypes.ClusterNodes — the single owner of the IP-offset arithmetic.
func BuildNodeList(cfg *config.Config) ([]NodeInfo, error) {
	enum, err := nodetypes.ClusterNodes(cfg)
	if err != nil {
		return nil, err
	}
	nodes := make([]NodeInfo, len(enum))
	for i, n := range enum {
		nodes[i] = NodeInfo{Name: n.Name(), Role: n.Role, IP: n.IP}
	}
	return nodes, nil
}
