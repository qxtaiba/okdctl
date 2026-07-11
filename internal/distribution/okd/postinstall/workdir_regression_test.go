package postinstall

import (
	"context"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// TestRemoveHAProxy_WorkDirRegression guards the workdir mispass: RemoveHAProxy
// must receive <workDir>/cluster-config where workDir is the okd-install
// directory (not the project root). The kubeconfig CA lives at
// <projectRoot>/okd-install/cluster-config/auth/kubeconfig, never at
// <projectRoot>/cluster-config/auth/kubeconfig.
func TestRemoveHAProxy_WorkDirRegression(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}
	}))
	t.Cleanup(srv.Close)

	port := srv.Listener.Addr().(*net.TCPAddr).Port

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	caData := base64.StdEncoding.EncodeToString(certPEM)
	kubeconfig := "clusters:\n- cluster:\n    certificate-authority-data: " + caData + "\n  name: test\n"

	projectRoot := t.TempDir()
	okdInstallDir := filepath.Join(projectRoot, phase.WorkDirName)
	correctClusterDir := phase.ClusterConfigDir(okdInstallDir)
	authDir := filepath.Join(correctClusterDir, "auth")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatalf("mkdir auth: %v", err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "kubeconfig"), []byte(kubeconfig), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}

	origConfig := haproxyConfigPath
	origPort := haproxyHealthPort
	origTimeout := haproxyVIPTimeout
	t.Cleanup(func() {
		haproxyConfigPath = origConfig
		haproxyHealthPort = origPort
		haproxyVIPTimeout = origTimeout
	})
	haproxyConfigPath = filepath.Join(t.TempDir(), "haproxy.cfg")
	haproxyHealthPort = port
	haproxyVIPTimeout = 1 * time.Second

	p := New(phase.WithExecutor(executor.New()), phase.WithLogger(logutil.NopLogger))

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
		wrongClusterDir := phase.ClusterConfigDir(projectRoot)
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
