package httputil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	c := New(5 * time.Second)
	if c.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", c.Timeout)
	}
	if c.Transport != nil {
		t.Errorf("New() should leave Transport nil for default (verified TLS)")
	}
	if c.CheckRedirect == nil {
		t.Error("CheckRedirect not installed; redirect cap policy is missing")
	}
}

func TestNewInsecure(t *testing.T) {
	c := NewInsecure(3 * time.Second)
	if c.Timeout != 3*time.Second {
		t.Errorf("Timeout = %v, want 3s", c.Timeout)
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport not *http.Transport: %T", c.Transport)
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig missing — insecure flag would be ignored")
	}
	if !tr.TLSClientConfig.InsecureSkipVerify {
		t.Errorf("InsecureSkipVerify = false; NewInsecure's whole purpose is to set this true")
	}

	// Defense-in-depth: New (secure variant) must not accidentally also
	// produce InsecureSkipVerify=true via shared-transport goofs.
	secure := New(3 * time.Second)
	if secure.Transport != nil {
		if st, ok := secure.Transport.(*http.Transport); ok && st.TLSClientConfig != nil {
			if st.TLSClientConfig.InsecureSkipVerify {
				t.Errorf("secure New() has InsecureSkipVerify=true — cross-contamination")
			}
		}
	}

	// Minor: ensure the tls config type is what we expect (guards against a
	// future refactor that swaps to a different package).
	var _ *tls.Config = tr.TLSClientConfig

	if c.CheckRedirect == nil {
		t.Error("CheckRedirect not installed on NewInsecure client")
	}
}

func TestNewWithCA(t *testing.T) {
	pool := x509.NewCertPool()
	c := NewWithCA(pool, 7*time.Second)
	if c.Timeout != 7*time.Second {
		t.Errorf("Timeout = %v, want 7s", c.Timeout)
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport not *http.Transport: %T", c.Transport)
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify must be false for CA-pinned client")
	}
	if tr.TLSClientConfig.RootCAs != pool {
		t.Error("RootCAs not set to provided pool")
	}
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want TLS 1.2 (%d)", tr.TLSClientConfig.MinVersion, tls.VersionTLS12)
	}
	if c.CheckRedirect == nil {
		t.Error("CheckRedirect not installed on NewWithCA client")
	}
}

func TestCapRedirects(t *testing.T) {
	mkReq := func(host, auth string) *http.Request {
		return &http.Request{
			URL:    &url.URL{Scheme: "https", Host: host, Path: "/x"},
			Header: http.Header{"Authorization": []string{auth}},
		}
	}
	mkVia := func(n int, host string) []*http.Request {
		via := make([]*http.Request, n)
		for i := range via {
			via[i] = &http.Request{URL: &url.URL{Scheme: "https", Host: host, Path: "/x"}}
		}
		return via
	}

	t.Run("same host below cap is allowed", func(t *testing.T) {
		req := mkReq("example.com", "Bearer x")
		if err := capRedirects(req, mkVia(1, "example.com")); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("cross host without auth is allowed", func(t *testing.T) {
		req := mkReq("other.com", "")
		if err := capRedirects(req, mkVia(1, "example.com")); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("cross host with auth is refused", func(t *testing.T) {
		req := mkReq("other.com", "Bearer x")
		err := capRedirects(req, mkVia(1, "example.com"))
		if err == nil {
			t.Fatal("expected refusal, got nil")
		}
	})

	t.Run("five redirects hits cap", func(t *testing.T) {
		req := mkReq("example.com", "")
		err := capRedirects(req, mkVia(5, "example.com"))
		if err == nil {
			t.Fatal("expected cap error, got nil")
		}
	})

	t.Run("four redirects below cap is allowed", func(t *testing.T) {
		req := mkReq("example.com", "")
		if err := capRedirects(req, mkVia(4, "example.com")); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("cap precedes cross-host check", func(t *testing.T) {
		// At cap with cross-host + auth, the cap fires first.
		req := mkReq("other.com", "Bearer x")
		err := capRedirects(req, mkVia(5, "example.com"))
		if err == nil {
			t.Fatal("expected cap error, got nil")
		}
	})
}

func TestKubeconfigCAPool(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	caData := base64.StdEncoding.EncodeToString(certPEM)

	kubeconfig := "clusters:\n- cluster:\n    certificate-authority-data: " + caData + "\n  name: test-cluster\n"
	dir := t.TempDir()
	kpath := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(kpath, []byte(kubeconfig), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}

	pool, err := KubeconfigCAPool(kpath)
	if err != nil {
		t.Fatalf("KubeconfigCAPool: %v", err)
	}
	if pool == nil {
		t.Fatal("pool is nil")
	}

	t.Run("missing file", func(t *testing.T) {
		_, err := KubeconfigCAPool(filepath.Join(dir, "nonexistent"))
		if err == nil {
			t.Error("expected error for missing file")
		}
	})

	t.Run("no clusters", func(t *testing.T) {
		p := filepath.Join(dir, "empty-kc")
		if err := os.WriteFile(p, []byte("clusters: []\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := KubeconfigCAPool(p)
		if err == nil {
			t.Error("expected error for kubeconfig with no certificate-authority-data")
		}
	})
}
