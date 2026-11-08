package workspace_test

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/workspace"
)

// TestLayoutContract pins the literal on-disk paths — a rename orphans
// existing workspaces.
func TestLayoutContract(t *testing.T) {
	root := "/proj"
	if got, want := workspace.WorkDir(root), "/proj/okd-install"; got != want {
		t.Errorf("WorkDir = %q, want %q", got, want)
	}
	if got, want := workspace.ClusterConfigDir(workspace.WorkDir(root)), "/proj/okd-install/cluster-config"; got != want {
		t.Errorf("ClusterConfigDir = %q, want %q", got, want)
	}
	if got, want := workspace.KubeconfigPath("/proj/okd-install/cluster-config"), "/proj/okd-install/cluster-config/auth/kubeconfig"; got != want {
		t.Errorf("KubeconfigPath = %q, want %q", got, want)
	}
}
