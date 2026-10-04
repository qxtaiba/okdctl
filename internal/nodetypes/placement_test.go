package nodetypes

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

func TestProxmoxPlacementPadsShortLists(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = "pve1"
	cfg.Provider.Proxmox.ControlPlaneNodes = []string{"pve2"}
	cfg.Provider.Proxmox.WorkerNodes = []string{"", "pve2"}
	for _, tc := range []struct {
		role  NodeRole
		index int
		want  string
	}{{RoleBootstrap, 0, "pve1"}, {RoleMaster, 0, "pve2"}, {RoleMaster, 1, "pve1"}, {RoleWorker, 0, "pve1"}, {RoleWorker, 1, "pve2"}, {RoleWorker, 2, "pve1"}} {
		if got := ProxmoxNode(cfg, tc.role, tc.index); got != tc.want {
			t.Fatalf("%s/%d: %s != %s", tc.role, tc.index, got, tc.want)
		}
	}
}
