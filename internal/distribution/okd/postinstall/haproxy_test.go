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
)

func setHAProxyConfigPath(t *testing.T, cfg string) {
	t.Helper()
	orig := haproxyConfigPath
	t.Cleanup(func() { haproxyConfigPath = orig })
	haproxyConfigPath = cfg
}

// startHealthzTLS serves /healthz over TLS; returns its port and a kubeconfig
// embedding the server CA.
func startHealthzTLS(t *testing.T) (port int, kubeconfig string) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}
	}))
	t.Cleanup(srv.Close)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	caData := base64.StdEncoding.EncodeToString(certPEM)
	kubeconfig = "clusters:\n- cluster:\n    certificate-authority-data: " + caData + "\n  name: test\n"
	return srv.Listener.Addr().(*net.TCPAddr).Port, kubeconfig
}

func pointHAProxyAtPort(t *testing.T, port int) {
	t.Helper()
	origPort := haproxyHealthPort
	origTimeout := haproxyVIPTimeout
	t.Cleanup(func() {
		haproxyHealthPort = origPort
		haproxyVIPTimeout = origTimeout
	})
	setHAProxyConfigPath(t, filepath.Join(t.TempDir(), "haproxy.cfg"))
	haproxyHealthPort = port
	haproxyVIPTimeout = 1 * time.Second
}

// writeAuthKubeconfig writes kubeconfig at clusterDir/auth/, matching
// workspace.KubeconfigPath's layout.
func writeAuthKubeconfig(t *testing.T, clusterDir, kubeconfig string) {
	t.Helper()
	authDir := filepath.Join(clusterDir, "auth")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatalf("mkdir auth: %v", err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "kubeconfig"), []byte(kubeconfig), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
}

// Backup name must match the glob cleanup/services.go sweeps at destroy.
func TestRemoveHAProxy_BackupCreated(t *testing.T) {
	cfgFile := filepath.Join(t.TempDir(), "haproxy.cfg")
	if err := os.WriteFile(cfgFile, []byte("frontend test\n  bind *:6443\n"), 0o644); err != nil {
		t.Fatalf("write stub config: %v", err)
	}
	setHAProxyConfigPath(t, cfgFile)

	p := newTestPhase(t)
	if err := p.RemoveHAProxy(context.Background(), "", t.TempDir()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	matches, err := filepath.Glob(phase.HAProxyBackupGlob(cfgFile))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one backup file matching %q; got %v", phase.HAProxyBackupGlob(cfgFile), matches)
	}

	got, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(got) != "frontend test\n  bind *:6443\n" {
		t.Errorf("backup content mismatch: %q", string(got))
	}
	if _, err := os.Stat(cfgFile); !os.IsNotExist(err) {
		t.Errorf("haproxy config still present after RemoveHAProxy")
	}
}

func TestRemoveHAProxy_ConfigRemoveAllError_DoesNotAbort(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses 0o555 perm; resilience branch unreachable as root")
	}
	protectedDir := filepath.Join(t.TempDir(), "protected")
	if err := os.Mkdir(protectedDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfgFile := filepath.Join(protectedDir, "haproxy.cfg")
	if err := os.WriteFile(cfgFile, []byte("# stub"), 0o644); err != nil {
		t.Fatalf("write stub config: %v", err)
	}
	if err := os.Chmod(protectedDir, 0o555); err != nil {
		t.Fatalf("chmod protected dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(protectedDir, 0o755) })
	setHAProxyConfigPath(t, cfgFile)

	p := newTestPhase(t)
	if err := p.RemoveHAProxy(context.Background(), "", t.TempDir()); err != nil {
		t.Fatalf("RemoveHAProxy must not abort on os.RemoveAll failure; got: %v", err)
	}
}

// CA-pool retrieval failure must hard-error, never fall back to InsecureSkipVerify.
func TestRemoveHAProxy_KubeVIPHealthcheck(t *testing.T) {
	port, kubeconfig := startHealthzTLS(t)

	dir := t.TempDir()
	writeAuthKubeconfig(t, dir, kubeconfig)
	pointHAProxyAtPort(t, port)

	p := newTestPhase(t)
	err := p.RemoveHAProxy(context.Background(), "127.0.0.1", dir)

	var networkErr *errtypes.NetworkError
	if errors.As(err, &networkErr) {
		t.Fatalf("VIP check returned NetworkError — CA-verified client not used: %v", err)
	}
	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Fatalf("expected ClusterError from oc hostname check; got: %T %v", err, err)
	}

	t.Run("hard_errors_when_kubeconfig_absent", func(t *testing.T) {
		emptyDir := t.TempDir()
		p2 := newTestPhase(t)
		err2 := p2.RemoveHAProxy(context.Background(), "127.0.0.1", emptyDir)

		var cluster2 *errtypes.ClusterError
		if !errors.As(err2, &cluster2) {
			t.Fatalf("expected ClusterError from missing-CA hard-error path; got: %T %v", err2, err2)
		}
		if !strings.Contains(cluster2.Msg, "kubeconfig CA unavailable") {
			t.Fatalf("ClusterError.Msg does not name the missing CA — fix may have regressed to InsecureSkipVerify fallback: %q", cluster2.Msg)
		}
	})
}
