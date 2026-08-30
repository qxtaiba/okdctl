package postinstall

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// Guards the workdir mispass: RemoveHAProxy must get <workDir>/cluster-config
// where workDir is okd-install, not the project root.
func TestRemoveHAProxy_WorkDirRegression(t *testing.T) {
	port, kubeconfig := startHealthzTLS(t)

	projectRoot := t.TempDir()
	correctClusterDir := workspace.ClusterConfigDir(workspace.WorkDir(projectRoot))
	writeAuthKubeconfig(t, correctClusterDir, kubeconfig)
	pointHAProxyAtPort(t, port)

	p := newTestPhase(t)

	t.Run("correct_workdir_passes_ca_check", func(t *testing.T) {
		err := p.RemoveHAProxy(context.Background(), "127.0.0.1", correctClusterDir)
		var clusterErr *errtypes.ClusterError
		if !errors.As(err, &clusterErr) {
			t.Fatalf("expected ClusterError from oc hostname check; got: %T %v", err, err)
		}
		if strings.Contains(clusterErr.Msg, "kubeconfig CA unavailable") {
			t.Fatalf("kubeconfig not found at correct path %q — workdir regression: %v",
				correctClusterDir, err)
		}
	})

	t.Run("wrong_workdir_fails_ca_check", func(t *testing.T) {
		wrongClusterDir := workspace.ClusterConfigDir(projectRoot)
		err := p.RemoveHAProxy(context.Background(), "127.0.0.1", wrongClusterDir)
		var clusterErr *errtypes.ClusterError
		if !errors.As(err, &clusterErr) {
			t.Fatalf("expected ClusterError; got: %T %v", err, err)
		}
		if !strings.Contains(clusterErr.Msg, "kubeconfig CA unavailable") {
			t.Fatalf("expected kubeconfig CA unavailable for wrong path %q; got: %v",
				wrongClusterDir, err)
		}
	})
}
