package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/proxmox"
	"github.com/qxtaiba/okdctl/internal/tui"
)

var doctorFixture = []proxmox.Finding{
	{Check: "version", Severity: "fail", Message: "proxmox 7.4 unsupported; need 8 or 9"},
	{Check: "memory", Severity: "warn", Message: "node pve1: planned 49152 MiB exceeds 65536 MiB total"},
	{Check: "bridge", Severity: "fail", Message: "bridge vmbr9 not found on node pve1"},
}

func TestPreflightInput(t *testing.T) {
	cases := []struct {
		name string
		px   *config.ProxmoxConfig
		want proxmox.PreflightInput
	}{
		{name: "nil provider block", px: nil, want: proxmox.PreflightInput{}},
		{
			name: "empty node with placement lists only",
			px: &config.ProxmoxConfig{
				Storage:           "local-lvm",
				ControlPlaneNodes: []string{"pve2"},
				WorkerNodes:       []string{"pve3"},
			},
			want: proxmox.PreflightInput{
				Nodes:      []string{"pve2", "pve3"},
				Storage:    "local-lvm",
				ISOStorage: "local-lvm",
				Bridge:     "vmbr0",
			},
		},
		{
			name: "fallbacks and node union",
			px: &config.ProxmoxConfig{
				Node:              "pve1",
				Storage:           "local-lvm",
				CPUType:           "x86-64-v3",
				HAEnabled:         true,
				ControlPlaneNodes: []string{"pve1", "pve2"},
				WorkerNodes:       []string{"pve3", "pve2"},
			},
			want: proxmox.PreflightInput{
				Nodes:      []string{"pve1", "pve2", "pve3"},
				Storage:    "local-lvm",
				ISOStorage: "local-lvm",
				Bridge:     "vmbr0",
				HA:         true,
				CPUModel:   "x86-64-v3",
			},
		},
		{
			name: "explicit iso storage and bridge",
			px: &config.ProxmoxConfig{
				Node:       "pve",
				Storage:    "ceph",
				ISOStorage: "local",
				Bridge:     "vmbr1",
			},
			want: proxmox.PreflightInput{
				Nodes:      []string{"pve"},
				Storage:    "ceph",
				ISOStorage: "local",
				Bridge:     "vmbr1",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{Provider: config.ProviderConfig{Type: config.ProviderProxmox, Proxmox: tc.px}}
			if got := preflightInput(cfg); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("preflightInput() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestPrintFindingsText(t *testing.T) {
	tui.SetColorProfileFor(&bytes.Buffer{})
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })

	var buf bytes.Buffer
	if err := printFindings(&buf, tui.FormatText, doctorFixture); err != nil {
		t.Fatalf("printFindings: %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 1+len(doctorFixture) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), 1+len(doctorFixture), buf.String())
	}
	for _, want := range []string{"CHECK", "SEVERITY", "MESSAGE"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("header %q missing %q", lines[0], want)
		}
	}
	for i, f := range doctorFixture {
		for _, want := range []string{f.Check, f.Severity, f.Message} {
			if !strings.Contains(lines[i+1], want) {
				t.Errorf("row %d %q missing %q", i, lines[i+1], want)
			}
		}
	}

	buf.Reset()
	if err := printFindings(&buf, tui.FormatText, nil); err != nil {
		t.Fatalf("printFindings(nil): %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "preflight passed" {
		t.Fatalf("printFindings(nil) = %q, want %q", got, "preflight passed")
	}
}

func TestPrintFindingsJSON(t *testing.T) {
	type row struct {
		Check    string `json:"check"`
		Severity string `json:"severity"`
		Message  string `json:"message"`
	}

	var buf bytes.Buffer
	if err := printFindings(&buf, tui.FormatJSON, doctorFixture); err != nil {
		t.Fatalf("printFindings: %v", err)
	}
	var got []row
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\n%s", err, buf.String())
	}
	want := make([]row, len(doctorFixture))
	for i, f := range doctorFixture {
		want[i] = row{f.Check, f.Severity, f.Message}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("json = %+v, want %+v", got, want)
	}

	buf.Reset()
	if err := printFindings(&buf, tui.FormatJSON, nil); err != nil {
		t.Fatalf("printFindings(nil): %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Fatalf("printFindings(nil) = %q, want %q", got, "[]")
	}
}

func TestFindingsError(t *testing.T) {
	cases := []struct {
		name     string
		findings []proxmox.Finding
		want     string
	}{
		{"empty", nil, ""},
		{"warn only", doctorFixture[1:2], ""},
		{"one fail", doctorFixture[:1], "preflight: 1 failing check"},
		{"two fails", doctorFixture, "preflight: 2 failing checks"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := findingsError(tc.findings)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("findingsError() = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tc.want {
				t.Fatalf("findingsError() = %v, want %q", err, tc.want)
			}
		})
	}
}

type doctorHarness struct {
	configPath string
	errOut     bytes.Buffer
}

func newDoctorHarness(t *testing.T) *doctorHarness {
	t.Helper()
	tui.SetColorProfileFor(&bytes.Buffer{})
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })

	h := &doctorHarness{configPath: filepath.Join(t.TempDir(), "okdctl.yaml")}
	if err := os.WriteFile(h.configPath, []byte(doctorConfigBody), 0o600); err != nil {
		t.Fatal(err)
	}
	origRun := runPreflight
	t.Cleanup(func() {
		runPreflight = origRun
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
		cfgFile = "okdctl.yaml"
		doctorOutput = tui.FormatText
	})
	return h
}

func (h *doctorHarness) run(t *testing.T, cfgPath, token string, findings []proxmox.Finding, runErr error, extra ...string) (string, error) {
	t.Helper()
	h.errOut.Reset()
	t.Setenv("PROXMOX_VE_API_TOKEN", token)
	t.Setenv("PROXMOX_VE_USERNAME", "")
	t.Setenv("PROXMOX_VE_PASSWORD", "")
	if os.Getenv("PROXMOX_VE_ENDPOINT") == "" {
		t.Setenv("PROXMOX_VE_ENDPOINT", "")
	}
	var gotCfg *config.Config
	var gotCreds *credentials.ProxmoxCredentials
	var gotToken string
	runPreflight = func(_ context.Context, cfg *config.Config, creds *credentials.ProxmoxCredentials) ([]proxmox.Finding, error) {
		gotCfg, gotCreds, gotToken = cfg, creds, string(creds.APIToken)
		return findings, runErr
	}
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&h.errOut)
	rootCmd.SetArgs(append([]string{"doctor", "--config", cfgPath, "--output", "text"}, extra...))
	err := rootCmd.Execute()
	if gotCfg != nil {
		h.assertSeam(t, gotCfg, gotCreds, gotToken, token)
	}
	return out.String(), err
}

func (h *doctorHarness) assertSeam(t *testing.T, cfg *config.Config, creds *credentials.ProxmoxCredentials, gotToken, token string) {
	t.Helper()
	if cfg.Provider.Proxmox.Node != "pve1" {
		t.Fatalf("seam received cfg %+v", cfg)
	}
	if creds == nil || creds.Endpoint == "" || gotToken != token {
		t.Fatalf("seam received creds %+v with token %q", creds, gotToken)
	}
	if creds.APIToken != nil {
		t.Fatal("token not zeroized after doctor returned")
	}
}

const doctorConfigBody = "schemaVersion: v2\nprovider:\n  type: proxmox\n  proxmox:\n    host: 192.0.2.10:8006\n    node: pve1\n    storage: local-lvm\n"

func TestDoctorCommand(t *testing.T) {
	h := newDoctorHarness(t)
	configPath := h.configPath
	run := h.run

	const token = "doctor@pve!ci=00000000-0000-0000-0000-000000000000"

	t.Run("fail exits non-zero after the table", func(t *testing.T) {
		out, err := run(t, configPath, token, doctorFixture, nil)
		if err == nil || err.Error() != "preflight: 2 failing checks" {
			t.Fatalf("err = %v, want %q", err, "preflight: 2 failing checks")
		}
		if !strings.Contains(out, "vmbr9 not found") || !strings.Contains(out, "CHECK") {
			t.Fatalf("table missing from stdout:\n%s", out)
		}
	})

	t.Run("warn only passes", func(t *testing.T) {
		out, err := run(t, configPath, token, doctorFixture[1:2], nil)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if !strings.Contains(out, "memory") || !strings.Contains(out, "warn") {
			t.Fatalf("warn row missing from stdout:\n%s", out)
		}
	})

	t.Run("no findings passes", func(t *testing.T) {
		out, err := run(t, configPath, token, nil, nil)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if strings.TrimSpace(out) != "preflight passed" {
			t.Fatalf("stdout = %q, want %q", out, "preflight passed")
		}
	})

	t.Run("json output is the bare array", func(t *testing.T) {
		out, err := run(t, configPath, token, doctorFixture, nil, "-o", "json")
		if err == nil {
			t.Fatal("err = nil, want failing-checks error")
		}
		var got []map[string]string
		if jsonErr := json.Unmarshal([]byte(out), &got); jsonErr != nil {
			t.Fatalf("stdout is not a JSON array: %v\n%s", jsonErr, out)
		}
		if len(got) != len(doctorFixture) || got[0]["check"] != "version" {
			t.Fatalf("json = %+v", got)
		}
	})

	t.Run("invalid output format is a usage error", func(t *testing.T) {
		_, err := run(t, configPath, token, nil, nil, "--output", "yaml")
		var ue *errtypes.UsageError
		if !errors.As(err, &ue) {
			t.Fatalf("err = %T %v, want *errtypes.UsageError", err, err)
		}
	})

	t.Run("preflight error propagates", func(t *testing.T) {
		_, err := run(t, configPath, token, nil, errors.New("read permissions: 401"))
		if err == nil || !strings.Contains(err.Error(), "read permissions: 401") {
			t.Fatalf("err = %v, want wrapped preflight error", err)
		}
	})

	t.Run("missing token names the env file and variable", func(t *testing.T) {
		_, err := run(t, configPath, "", nil, nil)
		if err == nil {
			t.Fatal("err = nil, want credential error")
		}
		for _, want := range []string{"okdctl.env", "PROXMOX_VE_API_TOKEN"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("err %q missing %q", err, want)
			}
		}
	})

	t.Run("http host without insecure_http is a config error", func(t *testing.T) {
		httpPath := filepath.Join(filepath.Dir(configPath), "insecure.yaml")
		httpBody := strings.Replace(doctorConfigBody, "host: 192.0.2.10:8006", "host: http://192.0.2.10:8006", 1)
		if err := os.WriteFile(httpPath, []byte(httpBody), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := run(t, httpPath, token, nil, nil)
		var ce *errtypes.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("err = %T %v, want *errtypes.ConfigError", err, err)
		}
		if strings.Contains(err.Error(), "PROXMOX_VE_API_TOKEN") {
			t.Fatalf("err %q blames the token for a host problem", err)
		}
		if !strings.Contains(h.errOut.String(), "insecure_http") {
			t.Fatalf("stderr does not carry the validator's insecure_http rule:\n%s", h.errOut.String())
		}
	})

	t.Run("http endpoint override without insecure_http is a config error", func(t *testing.T) {
		t.Setenv("PROXMOX_VE_ENDPOINT", "http://192.0.2.10:8006")
		_, err := run(t, configPath, token, nil, nil)
		var ce *errtypes.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("err = %T %v, want *errtypes.ConfigError", err, err)
		}
		for _, want := range []string{"http://", "insecure_http"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("err %q missing %q", err, want)
			}
		}
	})
}
