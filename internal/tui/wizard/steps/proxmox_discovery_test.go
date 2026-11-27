package steps

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/qxtaiba/okdctl/internal/credentials"
	okdproxmox "github.com/qxtaiba/okdctl/internal/proxmox"
)

const testToken = "root@pam!okdctl=s3cret"

func writeData(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

func newFakeProxmoxServer(t *testing.T, nodes http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	authed := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "PVEAPIToken="+testToken {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("GET /api2/json/nodes", authed(nodes))
	mux.HandleFunc("GET /api2/json/nodes/{node}/status", authed(func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("node") != "pve2" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeData(w, map[string]any{
			"cpuinfo": map[string]any{"cpus": 4, "model": "EPYC", "flags": "sse4_2 avx2"},
			"memory":  map[string]any{"total": 8589934592, "used": 1073741824},
		})
	}))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func twoNodes(w http.ResponseWriter, _ *http.Request) {
	writeData(w, []map[string]any{
		{"node": "pve1", "status": "offline", "maxcpu": 8, "maxmem": 17179869184},
		{"node": "pve2", "status": "online", "maxcpu": 2, "maxmem": 4294967296},
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
	if n := got.Nodes[0]; n.Name != "pve1" || n.Status != "offline" || n.CPUs != 8 || n.MemGB != 16 {
		t.Errorf("Nodes[0] = %+v; want the /nodes figures for an offline node", n)
	}
	if n := got.Nodes[1]; n.Name != "pve2" || n.Status != "online" || n.CPUs != 4 || n.MemGB != 8 || !n.CPUsKnown || !n.MemKnown {
		t.Errorf("Nodes[1] = %+v; want the node status figures for an online node", n)
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
			if msg := err.Error(); strings.Contains(msg, "s3cret") || strings.Contains(msg, "=wrong") {
				t.Fatalf("err = %v; leaks the api token", err)
			}
		})
	}
}

func TestDiscoverProxmox_WrongTokenIsErrPermission(t *testing.T) {
	server := newFakeProxmoxServer(t, twoNodes)
	_, err := discoverProxmox(t.Context(), testCredentials(server.URL, "root@pam!okdctl=wrong"))
	if !errors.Is(err, okdproxmox.ErrPermission) {
		t.Fatalf("err = %v; want the driver's ErrPermission in the chain", err)
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
		{"token without secret", testCredentials("https://pve:8006", "root@pam!okdctl"), "id=secret form"},
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

func TestClassifyError_ByType(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"permission", fmt.Errorf("get /nodes: %w", okdproxmox.ErrPermission), "authentication failed"},
		{"connection refused", &url.Error{Op: "Get", Err: &net.OpError{Op: "dial", Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED}}}, "connection refused"},
		{"no such host", &url.Error{Op: "Get", Err: &net.OpError{Op: "dial", Err: &net.DNSError{Name: "pve.invalid", IsNotFound: true}}}, "host not found"},
		{"dial timeout", &url.Error{Op: "Get", Err: &net.OpError{Op: "dial", Err: os.ErrDeadlineExceeded}}, "connection timed out"},
		{"context deadline", fmt.Errorf("get /nodes: %w", context.DeadlineExceeded), "connection timed out"},
		{"plain http on a tls port", &url.Error{Op: "Get", Err: tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"}}, "tls handshake failed"},
		{"tls alert", &url.Error{Op: "Get", Err: tls.AlertError(40)}, "tls handshake failed"},
		{"untrusted issuer", &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}, "ca certificate"},
		{"hostname mismatch", &tls.CertificateVerificationError{Err: x509.HostnameError{Certificate: &x509.Certificate{}, Host: "10.0.0.5"}}, "name on the certificate"},
		{"certificate expired", &tls.CertificateVerificationError{Err: x509.CertificateInvalidError{Reason: x509.Expired}}, "renew"},
		{"certificate invalid, other reason", &tls.CertificateVerificationError{Err: x509.CertificateInvalidError{Reason: x509.NotAuthorizedToSign}}, "tls certificate invalid"},
		{"unmapped", errors.New("status 401 unauthorized"), "connection failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyError(tc.err)
			if got == nil || !strings.Contains(got.Error(), tc.want) {
				t.Fatalf("classifyError(%v) = %v; want substring %q", tc.err, got, tc.want)
			}
			if !errors.Is(got, tc.err) {
				t.Errorf("classifyError(%v) = %v; lost the underlying error", tc.err, got)
			}
			if strings.Contains(got.Error(), "skip tls verify") {
				t.Errorf("classifyError(%v) = %v; must not nudge toward disabling tls verification", tc.err, got)
			}
		})
	}
}

func TestClassifyError_CancellationPassesThrough(t *testing.T) {
	err := fmt.Errorf("get /nodes: %w", context.Canceled)
	if got := classifyError(err); got.Error() != err.Error() {
		t.Fatalf("classifyError(canceled) = %v; want the error unchanged", got)
	}
}
