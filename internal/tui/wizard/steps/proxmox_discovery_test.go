package steps

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/credentials"
)

const testToken = "root@pam!okdctl=s3cret"

func writeData(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

// newFakeProxmoxServer answers /nodes only for the test token.
func newFakeProxmoxServer(t *testing.T, nodes http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api2/json/nodes", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "PVEAPIToken="+testToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		nodes(w, r)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func twoNodes(w http.ResponseWriter, _ *http.Request) {
	writeData(w, []map[string]any{
		{"node": "pve1", "status": "offline", "maxcpu": 8, "maxmem": 17179869184},
		{"node": "pve2", "status": "online", "maxcpu": 4, "maxmem": 8589934592},
	})
}

func testCredentials(endpoint, token string) *credentials.ProxmoxCredentials {
	return &credentials.ProxmoxCredentials{Endpoint: endpoint, APIToken: []byte(token)}
}

func TestDiscoverProxmox_Success(t *testing.T) {
	server := newFakeProxmoxServer(t, twoNodes)

	got, err := discoverProxmox(t.Context(), testCredentials(server.URL, testToken))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("len(Nodes) = %d; want 2", len(got.Nodes))
	}
	if got.Nodes[0].Name != "pve1" || got.Nodes[0].Status != "offline" {
		t.Errorf("Nodes[0] = %+v", got.Nodes[0])
	}
	if n := got.Nodes[1]; n.Name != "pve2" || n.Status != "online" || n.CPUs != 4 || n.MemGB != 8 || !n.CPUsKnown || !n.MemKnown {
		t.Errorf("Nodes[1] = %+v", n)
	}
}

func TestDiscoverProxmox_Failures(t *testing.T) {
	cases := []struct {
		name  string
		nodes http.HandlerFunc
		token string
		want  string
	}{
		{"no nodes found", func(w http.ResponseWriter, _ *http.Request) { writeData(w, []map[string]any{}) }, testToken, "no nodes found"},
		{"server error", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }, testToken, "connection failed"},
		{"wrong token", twoNodes, "root@pam!okdctl=wrong", "authentication failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newFakeProxmoxServer(t, tc.nodes)
			_, err := discoverProxmox(t.Context(), testCredentials(server.URL, tc.token))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v; want substring %q", err, tc.want)
			}
		})
	}
}

func TestDiscoverProxmox_ValidationBranches(t *testing.T) {
	cases := []struct {
		name  string
		creds *credentials.ProxmoxCredentials
		want  string
	}{
		{"missing endpoint", testCredentials("", testToken), "missing endpoint"},
		{"missing token", testCredentials("https://pve:8006", ""), "missing api token"},
		{"token without secret", testCredentials("https://pve:8006", "root@pam!okdctl"), "user@realm!tokenname=secret form"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := discoverProxmox(t.Context(), tc.creds)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v; want substring %q", err, tc.want)
			}
		})
	}
}

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"tls certificate error", errors.New("x509: certificate signed by unknown authority"), "tls handshake failed"},
		{"tls lowercase prefix", errors.New("tls: handshake failure"), "tls handshake failed"},
		{"connection refused", errors.New("dial tcp 10.0.0.1:8006: connect: connection refused"), "connection refused"},
		{"no such host", errors.New("dial tcp: lookup pve.invalid: no such host"), "host not found"},
		{"io timeout", errors.New("dial tcp 10.0.0.1:8006: i/o timeout"), "connection timed out"},
		{"context deadline", fmt.Errorf("get: %w", context.DeadlineExceeded), "connection timed out"},
		{"status 401", errors.New("status 401 unauthorized"), "check the api token"},
		{"authentication failure literal", errors.New("authentication failure"), "authentication failed"},
		{"unmapped error passthrough", errors.New("some other transport error"), "connection failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyError(tc.err)
			if got == nil || !strings.Contains(got.Error(), tc.want) {
				t.Errorf("classifyError(%v) = %v; want substring %q", tc.err, got, tc.want)
			}
			if strings.Contains(got.Error(), "skip tls verify") {
				t.Errorf("classifyError(%v) = %v; must not nudge toward disabling tls verification", tc.err, got)
			}
		})
	}
}

func TestClassifyError_TLSCausesGetDistinctSafeRemedies(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"untrusted issuer", &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}, "ca certificate"},
		{"hostname mismatch", &tls.CertificateVerificationError{Err: x509.HostnameError{Certificate: &x509.Certificate{}, Host: "10.0.0.5"}}, "name on the certificate"},
		{"certificate expired", &tls.CertificateVerificationError{Err: x509.CertificateInvalidError{Reason: x509.Expired}}, "renew"},
		{"certificate invalid, other reason", &tls.CertificateVerificationError{Err: x509.CertificateInvalidError{Reason: x509.NotAuthorizedToSign}}, "tls certificate invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyError(tc.err)
			if got == nil || !strings.Contains(got.Error(), tc.want) {
				t.Errorf("classifyError(%v) = %v; want substring %q", tc.err, got, tc.want)
			}
			if strings.Contains(got.Error(), "skip tls verify") {
				t.Errorf("classifyError(%v) = %v; must not nudge toward disabling tls verification", tc.err, got)
			}
			if !errors.Is(got, tc.err) {
				t.Errorf("classifyError(%v) = %v; lost the underlying error", tc.err, got)
			}
		})
	}
}
