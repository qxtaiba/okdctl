package provision

import (
	"errors"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestBuildNodeList(t *testing.T) {
	cases := []struct {
		name            string
		startIP         string
		masterCount     int
		workerCount     int
		cidr            string
		wantNames       []string
		wantIPs         []string
		wantErrContains string
	}{
		{
			name:        "1 master 0 workers no cidr",
			startIP:     "192.168.1.20",
			masterCount: 1,
			workerCount: 0,
			wantNames:   []string{"bootstrap", "master0"},
			wantIPs:     []string{"192.168.1.20", "192.168.1.21"},
		},
		{
			name:        "3 masters 2 workers with cidr",
			startIP:     "192.168.1.20",
			masterCount: 3,
			workerCount: 2,
			cidr:        "192.168.1.0/24",
			wantNames:   []string{"bootstrap", "master0", "master1", "master2", "worker0", "worker1"},
			wantIPs:     []string{"192.168.1.20", "192.168.1.21", "192.168.1.22", "192.168.1.23", "192.168.1.24", "192.168.1.25"},
		},
		{
			name:            "start ip outside cidr",
			startIP:         "192.168.2.10",
			masterCount:     3,
			workerCount:     2,
			cidr:            "192.168.1.0/24",
			wantErrContains: "static IP range",
		},
		{
			name:            "empty startIP with cidr returns config error",
			startIP:         "",
			masterCount:     1,
			cidr:            "192.168.1.0/24",
			wantErrContains: "static IP range",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				Topology: config.TopologyConfig{
					ControlPlane: config.NodeConfig{Count: tc.masterCount},
					Workers:      config.NodeConfig{Count: tc.workerCount},
				},
				Networking: config.NetworkingConfig{
					StaticIP:    config.StaticIPConfig{Start: tc.startIP},
					MachineCIDR: tc.cidr,
				},
			}
			nodes, err := BuildNodeList(cfg)
			if tc.wantErrContains != "" {
				if err == nil {
					t.Fatalf("want error containing %q; got nil", tc.wantErrContains)
				}
				if !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Errorf("error = %q; want substring %q", err.Error(), tc.wantErrContains)
				}
				var cfgErr *errtypes.ConfigError
				if !errors.As(err, &cfgErr) {
					t.Errorf("error type = %T; want *errtypes.ConfigError", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(nodes) != len(tc.wantNames) {
				t.Fatalf("len(nodes) = %d; want %d", len(nodes), len(tc.wantNames))
			}
			for i, node := range nodes {
				if node.Name != tc.wantNames[i] {
					t.Errorf("nodes[%d].Name = %q; want %q", i, node.Name, tc.wantNames[i])
				}
				if node.IP != tc.wantIPs[i] {
					t.Errorf("nodes[%d].IP = %q; want %q", i, node.IP, tc.wantIPs[i])
				}
			}
		})
	}
}
