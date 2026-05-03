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
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// TestRemoveHAProxy_EmptyVIPSkipsVerify exercises the vip=="" short-circuit
// — both verify blocks (vip + hostname) are gated behind `if vip != ""`,
// so the empty-vip path does no subprocess work beyond the (darwin-noop)
// systemctl probes and the os.RemoveAll on haproxyConfigPath. Verifies the
// new test seam (haproxyConfigPath var) works.
func TestRemoveHAProxy_EmptyVIPSkipsVerify(t *testing.T) {
	origConfig := haproxyConfigPath
	t.Cleanup(func() { haproxyConfigPath = origConfig })
	haproxyConfigPath = filepath.Join(t.TempDir(), "haproxy.cfg")

	p := New(executor.New(), logutil.NopLogger, "test")
	if err := p.RemoveHAProxy(context.Background(), "", t.TempDir()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRemoveHAProxy_KubeVIPHealthcheck verifies that RemoveHAProxy selects
// the CA-verified http.Client when the kubeconfig CA is present. The
// discriminant: a successful TLS handshake lets the VIP check pass so
// RemoveHAProxy advances to the oc hostname check, returning
// *errtypes.ClusterError. A TLS verification failure would return
// *errtypes.NetworkError from the VIP check instead.
func TestRemoveHAProxy_KubeVIPHealthcheck(t *testing.T) {
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

	dir := t.TempDir()
	authDir := filepath.Join(dir, "auth")
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

	p := New(executor.New(), logutil.NopLogger, "test")
	err := p.RemoveHAProxy(context.Background(), "127.0.0.1", dir)

	var networkErr *errtypes.NetworkError
	if errors.As(err, &networkErr) {
		t.Fatalf("VIP check returned NetworkError — CA-verified client not used: %v", err)
	}
	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Fatalf("expected ClusterError from oc hostname check; got: %T %v", err, err)
	}

	t.Run("insecure_fallback_when_kubeconfig_absent", func(t *testing.T) {
		emptyDir := t.TempDir()
		p2 := New(executor.New(), logutil.NopLogger, "test")
		err2 := p2.RemoveHAProxy(context.Background(), "127.0.0.1", emptyDir)

		var net2 *errtypes.NetworkError
		if errors.As(err2, &net2) {
			t.Fatalf("insecure fallback returned NetworkError from VIP check: %v", err2)
		}
		var cluster2 *errtypes.ClusterError
		if !errors.As(err2, &cluster2) {
			t.Fatalf("expected ClusterError from oc hostname check; got: %T %v", err2, err2)
		}
	})
}
