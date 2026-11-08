package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/model"
	"github.com/qxtaiba/okdctl/internal/proxmox"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

var doctorFixture = []proxmox.Finding{
	{Check: "version", Severity: "fail", Message: "proxmox 7.4 unsupported; need 8 or 9"},
	{Check: "memory", Severity: "warn", Message: "node pve1: planned 49152 MiB exceeds 65536 MiB total"},
	{Check: "bridge", Severity: "fail", Message: "bridge vmbr9 not found on node pve1"},
}

const (
	doctorPassed = "preflight passed (10 checks)"
	doctorToken  = "doctor@pve!ci=00000000-0000-0000-0000-000000000000"
)

func minimalConfigDir(t *testing.T) (cfg *config.Config, path string) {
	t.Helper()
	return config.MinimalConfig(), filepath.Join(t.TempDir(), "okdctl.yaml")
}

func addrs(s ...string) []netip.Addr {
	out := make([]netip.Addr, 0, len(s))
	for _, a := range s {
		out = append(out, netip.MustParseAddr(a))
	}
	return out
}

func TestPreflightInput(t *testing.T) {
	t.Run("record absent: preview identities carry no tag or macs", func(t *testing.T) {
		cfg, path := minimalConfigDir(t)
		got, err := preflightInput(cfg, path)
		if err != nil {
			t.Fatalf("preflightInput: %v", err)
		}
		want := proxmox.PreflightInput{
			Nodes:           []string{"pve"},
			Storage:         "local-lvm",
			ISOStorage:      "local",
			Bridge:          "vmbr0",
			CPUModel:        "host",
			VMIDs:           []int{6010, 6011, 6012},
			Addresses:       addrs("192.168.1.150", "192.168.1.151", "192.168.1.152", "192.168.1.158", "192.168.1.159"),
			MemoryMiBByNode: map[string]int{"pve": 3 * 16384},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("preflightInput() = %+v\nwant %+v", got, want)
		}
	})

	t.Run("record present: tag and macs come from the infra-id", func(t *testing.T) {
		cfg, path := minimalConfigDir(t)
		const infraID = "minimal-a1b2c3"
		rec := &model.Record{InfraID: infraID, Cluster: "minimal", Domain: "k8s.local", Created: time.Now(), Release: "4.22.0-okd-scos.10"}
		if err := model.SaveRecord(workspace.RecordPath(filepath.Dir(path)), rec); err != nil {
			t.Fatalf("SaveRecord: %v", err)
		}
		got, err := preflightInput(cfg, path)
		if err != nil {
			t.Fatalf("preflightInput: %v", err)
		}
		if got.Tag != infraID {
			t.Errorf("Tag = %q, want %q", got.Tag, infraID)
		}
		wantMACs := []string{model.MAC(infraID, 6010), model.MAC(infraID, 6011), model.MAC(infraID, 6012)}
		if !reflect.DeepEqual(got.MACs, wantMACs) {
			t.Errorf("MACs = %v, want %v", got.MACs, wantMACs)
		}
	})

	t.Run("iso storage falls back to storage", func(t *testing.T) {
		cfg, path := minimalConfigDir(t)
		cfg.Provider.Proxmox.ISOStorage = ""
		got, err := preflightInput(cfg, path)
		if err != nil {
			t.Fatalf("preflightInput: %v", err)
		}
		if got.ISOStorage != "local-lvm" {
			t.Fatalf("ISOStorage = %q, want %q", got.ISOStorage, "local-lvm")
		}
	})

	t.Run("memory sums per host and the provider block carries over", func(t *testing.T) {
		cfg, path := minimalConfigDir(t)
		cfg.Provider.Proxmox.Nodes = []string{"pve1", "pve2"}
		cfg.Provider.Proxmox.VLAN = 100
		cfg.Provider.Proxmox.Pool = "okd"
		cfg.Provider.Proxmox.HA = true
		cfg.Provider.Proxmox.CPUType = "x86-64-v3"
		cfg.Workers.Count = 2
		got, err := preflightInput(cfg, path)
		if err != nil {
			t.Fatalf("preflightInput: %v", err)
		}
		if got.VLAN != 100 || got.Pool != "okd" || !got.HA || got.CPUModel != "x86-64-v3" {
			t.Errorf("provider block not carried: vlan=%d pool=%q ha=%v cpu=%q", got.VLAN, got.Pool, got.HA, got.CPUModel)
		}
		wantMem := map[string]int{"pve1": 2*16384 + 8192, "pve2": 16384 + 8192}
		if !reflect.DeepEqual(got.MemoryMiBByNode, wantMem) {
			t.Errorf("MemoryMiBByNode = %v, want %v", got.MemoryMiBByNode, wantMem)
		}
		if len(got.VMIDs) != 5 || len(got.Addresses) != 7 {
			t.Errorf("got %d vmids and %d addresses, want 5 and 7", len(got.VMIDs), len(got.Addresses))
		}
	})

	t.Run("unbuildable inventory is a config error", func(t *testing.T) {
		cfg, path := minimalConfigDir(t)
		cfg.Networking.MachineCIDR = "bogus"
		_, err := preflightInput(cfg, path)
		var ce *errtypes.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("err = %T %v, want *errtypes.ConfigError", err, err)
		}
	})

	t.Run("broken record is a config error", func(t *testing.T) {
		cfg, path := minimalConfigDir(t)
		recPath := workspace.RecordPath(filepath.Dir(path))
		if err := os.MkdirAll(filepath.Dir(recPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(recPath, []byte("{broken"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := preflightInput(cfg, path)
		var ce *errtypes.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("err = %T %v, want *errtypes.ConfigError", err, err)
		}
	})
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
	if got := strings.TrimSpace(buf.String()); got != doctorPassed {
		t.Fatalf("printFindings(nil) = %q, want %q", got, doctorPassed)
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
	errOut bytes.Buffer
	seam   *proxmox.PreflightInput
}

// newDoctorHarness runs from a fresh directory with the relative okdctl.yaml
// every cli test uses, so LoadEnvFile sees one path for the whole process.
func newDoctorHarness(t *testing.T) *doctorHarness {
	t.Helper()
	tui.SetColorProfileFor(&bytes.Buffer{})
	t.Chdir(t.TempDir())
	if err := config.NewLoader().Save(config.MinimalConfig(), "okdctl.yaml"); err != nil {
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
		tui.SetColorProfileFor(&bytes.Buffer{})
	})
	return &doctorHarness{}
}

func (h *doctorHarness) run(t *testing.T, token string, findings []proxmox.Finding, runErr error, extra ...string) (string, error) {
	t.Helper()
	h.errOut.Reset()
	h.seam = nil
	t.Setenv("PROXMOX_VE_API_TOKEN", token)
	var gotCreds *credentials.ProxmoxCredentials
	var gotToken string
	runPreflight = func(_ context.Context, creds *credentials.ProxmoxCredentials, in *proxmox.PreflightInput) ([]proxmox.Finding, error) {
		h.seam, gotCreds, gotToken = in, creds, string(creds.APIToken)
		return findings, runErr
	}
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&h.errOut)
	rootCmd.SetArgs(append([]string{"doctor", "--config", "okdctl.yaml", "--output", "text"}, extra...))
	err := rootCmd.Execute()
	if h.seam != nil {
		h.assertSeam(t, gotCreds, gotToken, token)
	}
	return out.String(), err
}

func (h *doctorHarness) assertSeam(t *testing.T, creds *credentials.ProxmoxCredentials, gotToken, token string) {
	t.Helper()
	if !reflect.DeepEqual(h.seam.Nodes, []string{"pve"}) || len(h.seam.VMIDs) != 3 || h.seam.Tag != "" {
		t.Fatalf("seam received input %+v, want the minimal config's preview footprint", h.seam)
	}
	if creds == nil || creds.Endpoint == "" || gotToken != token {
		t.Fatalf("seam received creds %+v with token %q", creds, gotToken)
	}
	if creds.APIToken != nil {
		t.Fatal("token not zeroized after doctor returned")
	}
}

func TestDoctorCommand(t *testing.T) {
	h := newDoctorHarness(t)
	run := h.run
	const token = doctorToken

	t.Run("fail exits non-zero after the table", func(t *testing.T) {
		out, err := run(t, token, doctorFixture, nil)
		if err == nil || err.Error() != "preflight: 2 failing checks" {
			t.Fatalf("err = %v, want %q", err, "preflight: 2 failing checks")
		}
		if !strings.Contains(out, "vmbr9 not found") || !strings.Contains(out, "CHECK") {
			t.Fatalf("table missing from stdout:\n%s", out)
		}
	})

	t.Run("warn only passes", func(t *testing.T) {
		out, err := run(t, token, doctorFixture[1:2], nil)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if !strings.Contains(out, "memory") || !strings.Contains(out, "warn") {
			t.Fatalf("warn row missing from stdout:\n%s", out)
		}
	})

	t.Run("no findings passes", func(t *testing.T) {
		out, err := run(t, token, nil, nil)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if strings.TrimSpace(out) != doctorPassed {
			t.Fatalf("stdout = %q, want %q", out, doctorPassed)
		}
	})

	t.Run("json output is the bare array", func(t *testing.T) {
		out, err := run(t, token, doctorFixture, nil, "-o", "json")
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
		_, err := run(t, token, nil, nil, "--output", "yaml")
		var ue *errtypes.UsageError
		if !errors.As(err, &ue) {
			t.Fatalf("err = %T %v, want *errtypes.UsageError", err, err)
		}
	})

	t.Run("preflight error propagates", func(t *testing.T) {
		_, err := run(t, token, nil, errors.New("read permissions: 401"))
		if err == nil || !strings.Contains(err.Error(), "read permissions: 401") {
			t.Fatalf("err = %v, want wrapped preflight error", err)
		}
	})

	t.Run("rejected token is an auth error", func(t *testing.T) {
		_, err := run(t, token, nil, fmt.Errorf("read permissions: %w", proxmox.ErrPermission))
		var ae *errtypes.AuthError
		if !errors.As(err, &ae) {
			t.Fatalf("err = %T %v, want *errtypes.AuthError", err, err)
		}
		if !errors.Is(err, proxmox.ErrPermission) || !strings.Contains(err.Error(), "api token") {
			t.Fatalf("err = %v, want ErrPermission kept and the token named", err)
		}
	})

	t.Run("missing token names the env file and variable", func(t *testing.T) {
		_, err := run(t, "", nil, nil)
		var ae *errtypes.AuthError
		if !errors.As(err, &ae) {
			t.Fatalf("err = %T %v, want *errtypes.AuthError", err, err)
		}
		for _, want := range []string{"okdctl.env", "PROXMOX_VE_API_TOKEN"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("err %q missing %q", err, want)
			}
		}
		if h.seam != nil {
			t.Fatal("preflight ran without a token")
		}
	})
}

func TestDoctorCommandConfigErrors(t *testing.T) {
	h := newDoctorHarness(t)
	run := h.run

	t.Run("invalid config is a config error before credentials", func(t *testing.T) {
		if err := os.WriteFile("okdctl.yaml", []byte("schemaVersion: v3\ncluster:\n  name: Bad_Name\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := config.NewLoader().Save(config.MinimalConfig(), "okdctl.yaml"); err != nil {
				t.Fatal(err)
			}
		})
		_, err := run(t, "", nil, nil)
		var ce *errtypes.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("err = %T %v, want *errtypes.ConfigError", err, err)
		}
		if h.seam != nil || !strings.Contains(h.errOut.String(), "cluster.name") {
			t.Fatalf("seam=%v stderr=%q; want validation findings and no preflight", h.seam, h.errOut.String())
		}
	})

	t.Run("broken cluster record is a config error", func(t *testing.T) {
		recPath := workspace.RecordPath(".")
		if err := os.MkdirAll(filepath.Dir(recPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(recPath, []byte("{broken"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(recPath)) })
		_, err := run(t, doctorToken, nil, nil)
		var ce *errtypes.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("err = %T %v, want *errtypes.ConfigError", err, err)
		}
		if h.seam != nil {
			t.Fatal("preflight ran against a record doctor could not read")
		}
	})
}
