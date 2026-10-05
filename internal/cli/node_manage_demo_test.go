package cli

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/tui/wizard/lifecycle"
)

func TestDemoConfigMatchesDemoHooksFixture(t *testing.T) {
	cfg := demoConfig()
	if cfg.Cluster.Name != lifecycle.DemoClusterName {
		t.Fatalf("Cluster.Name = %q, want %q", cfg.Cluster.Name, lifecycle.DemoClusterName)
	}
	if cfg.Topology.ControlPlane.Count == 0 || cfg.Topology.Workers.Count == 0 {
		t.Fatalf("demo config must retain a real node topology: %+v", cfg.Topology)
	}
}
