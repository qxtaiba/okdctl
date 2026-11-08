package proxmox

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestNormalizeEndpoint(t *testing.T) {
	cases := map[string]string{
		"https://pve:8006/":    "https://pve:8006",
		"pve.example.com":      "https://pve.example.com",
		"pve.example.com/":     "https://pve.example.com",
		"http://10.0.0.1:8006": "http://10.0.0.1:8006",
	}
	for in, want := range cases {
		if got := normalizeEndpoint(in); got != want {
			t.Errorf("normalizeEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewProxmoxClientWarnsOnInsecureOnce(t *testing.T) {
	insecureTLSWarnOnce = sync.Once{}
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	build := func() {
		t.Helper()
		if _, _, err := newProxmoxClient("https://pve:8006", []byte("user@pam!t=secret"), true); err != nil {
			t.Fatalf("newProxmoxClient: %v", err)
		}
	}

	build()
	if !strings.Contains(buf.String(), "tls verification disabled") {
		t.Fatalf("expected insecure tls warning, log = %q", buf.String())
	}
	buf.Reset()
	build()
	if strings.Contains(buf.String(), "tls verification disabled") {
		t.Errorf("warning must fire once per process, log = %q", buf.String())
	}
}

func TestBadRequestRendersBodyOnce(t *testing.T) {
	const detail = `{"vmid":"invalid format - value does not look like a valid VM ID"}`
	f := newFakeAPI(t, map[string]http.HandlerFunc{
		"POST /nodes/pve/qemu":           jsonHandler(http.StatusBadRequest, `{"data":null,"errors":`+detail+`}`),
		"GET /nodes/pve/qemu/105/config": missingConfig(105),
	})
	_, err := f.driver(t).CreateVM(t.Context(), "pve", newSpec(105, "grappleberry"))
	if err == nil || strings.Count(err.Error(), "invalid format") != 1 {
		t.Fatalf("CreateVM() error = %v, want the 400 detail exactly once", err)
	}
}
