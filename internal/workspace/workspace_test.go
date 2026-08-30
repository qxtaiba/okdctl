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
	if got, want := workspace.TerraformEnvDir(root, "production"), "/proj/infrastructure/terraform/environments/production"; got != want {
		t.Errorf("TerraformEnvDir = %q, want %q", got, want)
	}
	if got, want := workspace.TerraformEnvDir(root, ""), "/proj/infrastructure/terraform/environments"; got != want {
		t.Errorf("TerraformEnvDir empty env = %q, want %q", got, want)
	}
	if got, want := workspace.TerraformModuleDir(root), "/proj/infrastructure/terraform/modules/proxmox-okd"; got != want {
		t.Errorf("TerraformModuleDir = %q, want %q", got, want)
	}
	if workspace.DefaultTerraformEnv != "production" {
		t.Errorf("DefaultTerraformEnv = %q, want %q", workspace.DefaultTerraformEnv, "production")
	}
	if workspace.BootstrapStateSentinelFile != "bootstrap-state.auto.tfvars.json" {
		t.Errorf("BootstrapStateSentinelFile = %q", workspace.BootstrapStateSentinelFile)
	}
}
