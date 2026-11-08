package workspace_test

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/workspace"
)

func TestLayoutContract(t *testing.T) {
	const root = "/proj"
	tests := []struct {
		name string
		fn   func(string) string
		want string
	}{
		{"ClusterDir", workspace.ClusterDir, "/proj/cluster"},
		{"RecordPath", workspace.RecordPath, "/proj/cluster/record.json"},
		{"AuthDir", workspace.AuthDir, "/proj/cluster/auth"},
		{"KubeconfigPath", workspace.KubeconfigPath, "/proj/cluster/auth/kubeconfig"},
		{"KubeadminPasswordPath", workspace.KubeadminPasswordPath, "/proj/cluster/auth/kubeadmin-password"},
		{"StateFilePath", workspace.StateFilePath, "/proj/cluster/.openshift_install_state.json"},
		{"ISOPath", workspace.ISOPath, "/proj/cluster/agent.x86_64.iso"},
		{"ExtraManifestsDir", workspace.ExtraManifestsDir, "/proj/cluster/openshift"},
		{"HandoffManifestsDir", workspace.HandoffManifestsDir, "/proj/handoff/manifests"},
		{"HandoffSecretsDir", workspace.HandoffSecretsDir, "/proj/handoff/secrets"},
		{"BackupsDir", workspace.BackupsDir, "/proj/backups"},
		{"BinDir", workspace.BinDir, "/proj/bin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn(root); got != tt.want {
				t.Errorf("%s(%q) = %q, want %q", tt.name, root, got, tt.want)
			}
		})
	}
}
