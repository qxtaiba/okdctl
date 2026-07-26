package nodetypes

import (
	"errors"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func clusterCfg(startIP, cidr string, masters, workers int) *config.Config {
	return &config.Config{
		Topology: config.TopologyConfig{
			ControlPlane: config.NodeConfig{Count: masters},
			Workers:      config.NodeConfig{Count: workers},
		},
		Networking: config.NetworkingConfig{
			StaticIP:    config.StaticIPConfig{Start: startIP},
			MachineCIDR: cidr,
		},
	}
}

func TestClusterNodes_OffsetsAndCounts(t *testing.T) {
	nodes, err := ClusterNodes(clusterCfg("192.168.1.20", "192.168.1.0/24", 3, 2))
	if err != nil {
		t.Fatalf("ClusterNodes: %v", err)
	}

	want := []struct {
		name string
		role NodeRole
		ip   string
	}{
		{"bootstrap", RoleBootstrap, "192.168.1.20"},
		{"master0", RoleMaster, "192.168.1.21"},
		{"master1", RoleMaster, "192.168.1.22"},
		{"master2", RoleMaster, "192.168.1.23"},
		{"worker0", RoleWorker, "192.168.1.24"},
		{"worker1", RoleWorker, "192.168.1.25"},
	}
	if len(nodes) != len(want) {
		t.Fatalf("len(nodes) = %d; want %d", len(nodes), len(want))
	}
	for i, w := range want {
		if got := nodes[i].Name(); got != w.name {
			t.Errorf("nodes[%d].Name() = %q; want %q", i, got, w.name)
		}
		if nodes[i].Role != w.role {
			t.Errorf("nodes[%d].Role = %q; want %q", i, nodes[i].Role, w.role)
		}
		if nodes[i].IP != w.ip {
			t.Errorf("nodes[%d].IP = %q; want %q", i, nodes[i].IP, w.ip)
		}
	}
}

func TestClusterNodes_InvalidBaseIPPropagates(t *testing.T) {
	// No CIDR configured, so the range pre-check is skipped and the failure
	// must surface from the first per-node offset calculation.
	_, err := ClusterNodes(clusterCfg("not-an-ip", "", 1, 1))
	if err == nil {
		t.Fatal("want error for invalid base IP, got nil")
	}
	if !strings.Contains(err.Error(), "calculate master0 IP") {
		t.Errorf("error = %q; want the single-sourced calculate-IP wrap", err)
	}
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Errorf("error type = %T; want *errtypes.ConfigError", err)
	}
}

func TestClusterNodes_RangeOutsideCIDR(t *testing.T) {
	_, err := ClusterNodes(clusterCfg("192.168.1.254", "192.168.1.0/24", 3, 2))
	if err == nil {
		t.Fatal("want error for range overflowing the CIDR, got nil")
	}
	if !strings.Contains(err.Error(), "static IP range does not fit in machine CIDR") {
		t.Errorf("error = %q; want CIDR-fit message", err)
	}
}
