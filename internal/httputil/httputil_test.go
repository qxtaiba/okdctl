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

func TestClientFactories(t *testing.T) {
	pool := x509.NewCertPool()
	cases := []struct {
		name         string
		client       *http.Client
		wantTimeout  time.Duration
		nilTransport bool
		wantInsecure bool
		checkTLS     func(t *testing.T, cfg *tls.Config)
	}{
		{
			name:         "New leaves Transport nil for verified TLS",
			client:       New(5 * time.Second),
			wantTimeout:  5 * time.Second,
			nilTransport: true,
		},
		{
			name:         "NewInsecure skips verification",
			client:       NewInsecure(3 * time.Second),
			wantTimeout:  3 * time.Second,
			wantInsecure: true,
		},
		{
			name:         "NewOptionalInsecure true skips verification",
			client:       NewOptionalInsecure(true, 3*time.Second),
			wantTimeout:  3 * time.Second,
			wantInsecure: true,
		},
		{
			name:        "NewOptionalInsecure false keeps verification on",
			client:      NewOptionalInsecure(false, time.Second),
			wantTimeout: time.Second,
		},
		{
			name:        "NewWithCA pins pool and TLS 1.2",
			client:      NewWithCA(pool, 7*time.Second),
			wantTimeout: 7 * time.Second,
			checkTLS: func(t *testing.T, cfg *tls.Config) {
				t.Helper()
				if cfg.RootCAs != pool {
					t.Error("RootCAs not set to provided pool")
				}
				if cfg.MinVersion != tls.VersionTLS12 {
					t.Errorf("MinVersion = %d, want TLS 1.2 (%d)", cfg.MinVersion, tls.VersionTLS12)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.client.Timeout != tc.wantTimeout {
				t.Errorf("Timeout = %v, want %v", tc.client.Timeout, tc.wantTimeout)
			}
			if tc.client.CheckRedirect == nil {
				t.Error("CheckRedirect not installed; redirect cap policy is missing")
			}
			if tc.nilTransport {
				if tc.client.Transport != nil {
					t.Errorf("Transport = %T, want nil for default (verified TLS)", tc.client.Transport)
				}
				return
			}
			tr, ok := tc.client.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("Transport not *http.Transport: %T", tc.client.Transport)
			}
			if tr.TLSClientConfig == nil {
				t.Fatal("TLSClientConfig missing — verification mode would be ignored")
			}
			if tr.TLSClientConfig.InsecureSkipVerify != tc.wantInsecure {
				t.Errorf("InsecureSkipVerify = %v, want %v", tr.TLSClientConfig.InsecureSkipVerify, tc.wantInsecure)
			}
			if tc.checkTLS != nil {
				tc.checkTLS(t, tr.TLSClientConfig)
			}
		})
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
