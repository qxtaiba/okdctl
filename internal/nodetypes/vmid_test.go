package nodetypes

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

func TestVMID(t *testing.T) {
	cfg := &config.Config{Topology: config.TopologyConfig{VMIDBase: 500}}
	cases := []struct {
		role  NodeRole
		index int
		want  int
	}{
		{RoleBootstrap, 0, 500},
		{RoleMaster, 0, 510},
		{RoleMaster, 2, 512},
		{RoleWorker, 0, 600},
		{RoleWorker, 3, 603},
	}
	for _, tc := range cases {
		if got := VMID(cfg, tc.role, tc.index); got != tc.want {
			t.Errorf("VMID(%s, %d) = %d; want %d", tc.role, tc.index, got, tc.want)
		}
	}

	zeroBase := &config.Config{}
	if got := VMID(zeroBase, RoleMaster, 1); got != config.DefaultVMIDBase+11 {
		t.Errorf("zero base master1 = %d; want default base + 11", got)
	}
}
