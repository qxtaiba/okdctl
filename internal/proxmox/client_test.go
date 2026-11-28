package proxmox

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
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
	t.Cleanup(logutil.Redirect(&buf))

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

func TestNewProxmoxClientWarnsOnPlaintextHTTPOnce(t *testing.T) {
	plaintextHTTPWarnOnce = sync.Once{}
	var buf bytes.Buffer
	t.Cleanup(logutil.Redirect(&buf))

	for _, endpoint := range []string{"http://pve:8006", "http://pve:8006"} {
		if _, _, err := newProxmoxClient(endpoint, []byte("user@pam!t=secret"), false); err != nil {
			t.Fatalf("newProxmoxClient(%q): %v", endpoint, err)
		}
	}
	if log := buf.String(); strings.Count(log, "plaintext http") != 1 || !strings.Contains(log, "endpoint=http://pve:8006") {
		t.Fatalf("want one plaintext warning naming the endpoint, log = %q", log)
	}
	buf.Reset()
	if _, _, err := newProxmoxClient("https://pve:8006", []byte("user@pam!t=secret"), false); err != nil {
		t.Fatalf("newProxmoxClient: %v", err)
	}
	if strings.Contains(buf.String(), "plaintext http") {
		t.Errorf("https endpoint must not warn, log = %q", buf.String())
	}
}

func TestBadRequestRendersBodyOnce(t *testing.T) {
	const detail = `{"vmid":"invalid format - value does not look like a valid VM ID"}`
	f := newFakeAPI(t, map[string]http.HandlerFunc{
		"POST /nodes/pve/qemu":                     jsonHandler(http.StatusBadRequest, `{"data":null,"errors":`+detail+`}`),
		"GET /nodes/pve/qemu/105/config?current=1": missingConfig(105),
	})
	_, err := f.driver(t).CreateVM(t.Context(), "pve", newSpec(105, "grappleberry"))
	if err == nil || strings.Count(err.Error(), "invalid format") != 1 {
		t.Fatalf("CreateVM() error = %v, want the 400 detail exactly once", err)
	}
}

func TestGatewayFailuresAreErrors(t *testing.T) {
	ref105 := VMRef{Node: "pve", VMID: 105}
	cases := []struct {
		name string
		key  string
		h    http.HandlerFunc
		run  func(Driver) error
		want string
	}{
		{
			name: "set ha on 503 with empty body",
			key:  "POST /cluster/ha/resources",
			h:    func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) },
			run:  func(d Driver) error { return d.SetHA(t.Context(), ref105, HASpec{State: "started"}) },
			want: "503",
		},
		{
			name: "clear ha on 404 html",
			key:  "DELETE /cluster/ha/resources/vm:101",
			h: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("<html>not here</html>"))
			},
			run:  func(d Driver) error { return d.ClearHA(t.Context(), ref101) },
			want: "404",
		},
		{
			name: "start on 503 with null data",
			key:  "POST /nodes/pve/qemu/101/status/start",
			h:    jsonHandler(http.StatusServiceUnavailable, `{"data":null,"message":"pveproxy restarting"}`),
			run: func(d Driver) error {
				task, err := d.Start(t.Context(), ref101)
				if err == nil && task.UPID == "" {
					return errors.New("empty upid with nil error")
				}
				return err
			},
			want: "pveproxy restarting",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t, map[string]http.HandlerFunc{
				tc.key:                             tc.h,
				"GET /cluster/ha/resources/vm:105": pveErrorHandler("no such resource 'vm:105'"),
			})
			err := tc.run(f.driver(t))
			if err == nil || !strings.Contains(err.Error(), tc.want) || errors.Is(err, ErrNotFound) {
				t.Fatalf("error = %v, want a generic error carrying %q", err, tc.want)
			}
		})
	}
}

func TestTaskFailedErrorNamesTheTask(t *testing.T) {
	err := &TaskFailed{UPID: upidStart}
	if !strings.Contains(err.Error(), upidStart) {
		t.Fatalf("Error() = %q, want it to name the task when the exit status is empty", err.Error())
	}
}
