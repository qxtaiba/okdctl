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

// TestRemoveHAProxy_EmptyVIPSkipsVerify exercises the vip=="" short-circuit
// — both verify blocks (vip + hostname) are gated behind `if vip != ""`,
// so the empty-vip path does no subprocess work beyond the (darwin-noop)
// systemctl probes and the os.RemoveAll on haproxyConfigPath. Verifies the
// new test seam (haproxyConfigPath var) works.
func TestRemoveHAProxy_EmptyVIPSkipsVerify(t *testing.T) {
	origConfig := haproxyConfigPath
	t.Cleanup(func() { haproxyConfigPath = origConfig })
	haproxyConfigPath = filepath.Join(t.TempDir(), "haproxy.cfg")

	p := New("test", phase.WithExecutor(executor.New()), phase.WithLogger(logutil.NopLogger))
	if err := p.RemoveHAProxy(context.Background(), "", t.TempDir()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRemoveHAProxy_HappyPath_ConfigFileRemoved verifies that RemoveHAProxy
// deletes haproxyConfigPath when the file exists and vip is empty.
func TestRemoveHAProxy_HappyPath_ConfigFileRemoved(t *testing.T) {
	cfgFile := filepath.Join(t.TempDir(), "haproxy.cfg")
	if err := os.WriteFile(cfgFile, []byte("# stub"), 0o644); err != nil {
		t.Fatalf("write stub config: %v", err)
	}

	origConfig := haproxyConfigPath
	t.Cleanup(func() { haproxyConfigPath = origConfig })
	haproxyConfigPath = cfgFile

	p := New("test", phase.WithExecutor(executor.New()), phase.WithLogger(logutil.NopLogger))
	if err := p.RemoveHAProxy(context.Background(), "", t.TempDir()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(cfgFile); !os.IsNotExist(err) {
		t.Errorf("haproxy config still present after RemoveHAProxy")
	}
}

// TestRemoveHAProxy_BackupCreated verifies that RemoveHAProxy writes a
// timestamped backup matching the glob that cleanup/services.go uses to
// sweep haproxy backup files during destroy.
func TestRemoveHAProxy_BackupCreated(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "haproxy.cfg")
	if err := os.WriteFile(cfgFile, []byte("frontend test\n  bind *:6443\n"), 0o644); err != nil {
		t.Fatalf("write stub config: %v", err)
	}

	origConfig := haproxyConfigPath
	t.Cleanup(func() { haproxyConfigPath = origConfig })
	haproxyConfigPath = cfgFile

	p := New("test", phase.WithExecutor(executor.New()), phase.WithLogger(logutil.NopLogger))
	if err := p.RemoveHAProxy(context.Background(), "", t.TempDir()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	matches, err := filepath.Glob(cfgFile + ".backup.*")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one backup file matching %q; got %v", cfgFile+".backup.*", matches)
	}

	got, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(got) != "frontend test\n  bind *:6443\n" {
		t.Errorf("backup content mismatch: %q", string(got))
	}
}

// TestRemoveHAProxy_ConfigRemoveAllError_DoesNotAbort verifies that an
// os.RemoveAll failure (unwritable parent dir) is logged as a warning and
// does not cause RemoveHAProxy to return an error.
func TestRemoveHAProxy_ConfigRemoveAllError_DoesNotAbort(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses 0o555 perm; resilience branch unreachable as root")
	}
	parentDir := t.TempDir()
	protectedDir := filepath.Join(parentDir, "protected")
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

	origConfig := haproxyConfigPath
	t.Cleanup(func() { haproxyConfigPath = origConfig })
	haproxyConfigPath = cfgFile

	p := New("test", phase.WithExecutor(executor.New()), phase.WithLogger(logutil.NopLogger))
	if err := p.RemoveHAProxy(context.Background(), "", t.TempDir()); err != nil {
		t.Fatalf("RemoveHAProxy must not abort on os.RemoveAll failure; got: %v", err)
	}
}

// TestRemoveHAProxy_KubeVIPHealthcheck verifies CA-pool handling in
// RemoveHAProxy. With kubeconfig present, the CA-verified http.Client lets
// the VIP /healthz check succeed and RemoveHAProxy advances to the oc
// hostname check, returning *errtypes.ClusterError. With kubeconfig absent,
// CA-pool retrieval fails and RemoveHAProxy MUST hard-error with
// *errtypes.ClusterError naming the missing CA — never fall back to
// InsecureSkipVerify (sec:761e5126).
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

	p := New("test", phase.WithExecutor(executor.New()), phase.WithLogger(logutil.NopLogger))
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
		p2 := New("test", phase.WithExecutor(executor.New()), phase.WithLogger(logutil.NopLogger))
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
