package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	doctorPassed = "preflight passed"
	doctorToken  = "doctor@pve!ci=00000000-0000-0000-0000-000000000000"
)

func minimalConfigDir(t *testing.T) (cfg *config.Config, path string) {
	t.Helper()
	return config.MinimalConfig(), filepath.Join(t.TempDir(), "okdctl.yaml")
}

var addr = netip.MustParseAddr

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
			VMIDs:           []int{6010, 6011, 6012},
			NodeAddresses:   map[netip.Addr]int{addr("192.168.1.150"): 6010, addr("192.168.1.151"): 6011, addr("192.168.1.152"): 6012},
			VIPs:            []netip.Addr{addr("192.168.1.158"), addr("192.168.1.159")},
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
		cfg.Workers.Count = 2
		got, err := preflightInput(cfg, path)
		if err != nil {
			t.Fatalf("preflightInput: %v", err)
		}
		if got.VLAN != 100 || got.Pool != "okd" || !got.HA {
			t.Errorf("provider block not carried: vlan=%d pool=%q ha=%v", got.VLAN, got.Pool, got.HA)
		}
		wantMem := map[string]int{"pve1": 2*16384 + 8192, "pve2": 16384 + 8192}
		if !reflect.DeepEqual(got.MemoryMiBByNode, wantMem) {
			t.Errorf("MemoryMiBByNode = %v, want %v", got.MemoryMiBByNode, wantMem)
		}
		if len(got.VMIDs) != 5 || len(got.NodeAddresses) != 5 || len(got.VIPs) != 2 {
			t.Errorf("got %d vmids, %d node addresses and %d vips, want 5, 5 and 2", len(got.VMIDs), len(got.NodeAddresses), len(got.VIPs))
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

type doctorHarness struct {
	errOut bytes.Buffer
	seam   *proxmox.PreflightInput
	driver proxmox.Driver
}

func newDoctorHarness(t *testing.T) *doctorHarness {
	t.Helper()
	tui.SetColorProfileFor(&bytes.Buffer{})
	t.Chdir(t.TempDir())
	if err := config.NewLoader().Save(config.MinimalConfig(), "okdctl.yaml"); err != nil {
		t.Fatal(err)
	}
	origRun, origOpen := runPreflight, openDriver
	t.Cleanup(func() {
		runPreflight, openDriver = origRun, origOpen
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
	runPreflight = func(_ context.Context, d proxmox.Driver, in *proxmox.PreflightInput) ([]proxmox.Finding, error) {
		h.seam, h.driver = in, d
		return findings, runErr
	}
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&h.errOut)
	rootCmd.SetArgs(append([]string{"doctor", "--config", "okdctl.yaml", "--output", "text"}, extra...))
	err := rootCmd.Execute()
	if h.seam != nil {
		h.assertSeam(t)
	}
	return out.String(), err
}

func (h *doctorHarness) assertSeam(t *testing.T) {
	t.Helper()
	if !reflect.DeepEqual(h.seam.Nodes, []string{"pve"}) || len(h.seam.VMIDs) != 3 || h.seam.Tag != "" {
		t.Fatalf("seam received input %+v, want the minimal config's preview footprint", h.seam)
	}
	if h.driver == nil {
		t.Fatal("seam received no driver")
	}
}

func TestDoctorReadsTheSidecarToken(t *testing.T) {
	h := newDoctorHarness(t)
	t.Setenv("PROXMOX_VE_API_TOKEN", "")
	saved := &credentials.ProxmoxCredentials{APIToken: []byte(doctorToken)}
	if err := credentials.WriteEnvFile("okdctl.env", saved); err != nil {
		t.Fatal(err)
	}
	var gotToken string
	openDriver = func(creds *credentials.ProxmoxCredentials) (proxmox.Driver, error) {
		gotToken = string(creds.APIToken)
		return nil, nil
	}
	runPreflight = func(context.Context, proxmox.Driver, *proxmox.PreflightInput) ([]proxmox.Finding, error) {
		return nil, nil
	}
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(&h.errOut)
	rootCmd.SetArgs([]string{"doctor", "--config", "okdctl.yaml"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if gotToken != doctorToken {
		t.Fatalf("preflight received token %q, want the sidecar's", gotToken)
	}
}

func TestOpenDriverZeroizesToken(t *testing.T) {
	for name, endpoint := range map[string]string{"valid": "https://pve:8006", "empty": ""} {
		creds := &credentials.ProxmoxCredentials{Endpoint: endpoint, APIToken: []byte(doctorToken)}
		_, err := openDriver(creds)
		if (err != nil) != (name == "empty") {
			t.Fatalf("openDriver(%s) err = %v", name, err)
		}
		if creds.APIToken != nil {
			t.Fatalf("openDriver(%s) left the token in place", name)
		}
	}
}

func TestDoctorCommand(t *testing.T) {
	h := newDoctorHarness(t)
	run := h.run
	origOpen := openDriver
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

	t.Run("transport failure is a network error", func(t *testing.T) {
		_, err := run(t, token, nil, errors.New("read permissions: dial tcp 127.0.0.1:1: connect: connection refused"))
		var ne *errtypes.NetworkError
		if !errors.As(err, &ne) || exitCodeFor(err) != 3 {
			t.Fatalf("err = %T %v (exit %d), want *errtypes.NetworkError exiting 3", err, err, exitCodeFor(err))
		}
		if !strings.Contains(err.Error(), "connection refused") {
			t.Fatalf("err = %v, want the transport cause kept", err)
		}
	})

	t.Run("json transport failure leaves an empty array on stdout", func(t *testing.T) {
		out, err := run(t, token, nil, errors.New("dial tcp: connection refused"), "-o", "json")
		if err == nil || strings.TrimSpace(out) != "[]" {
			t.Fatalf("stdout = %q err = %v, want [] and the network error", out, err)
		}
	})

	t.Run("json driver failure leaves an empty array on stdout", func(t *testing.T) {
		openDriver = func(*credentials.ProxmoxCredentials) (proxmox.Driver, error) {
			return nil, &errtypes.ConfigError{Msg: "connect proxmox"}
		}
		t.Cleanup(func() { openDriver = origOpen })
		out, err := run(t, token, nil, nil, "-o", "json")
		var ce *errtypes.ConfigError
		if !errors.As(err, &ce) || strings.TrimSpace(out) != "[]" {
			t.Fatalf("stdout = %q err = %v, want [] and the config error", out, err)
		}
	})

	t.Run("cancellation keeps its identity through the network error", func(t *testing.T) {
		_, err := run(t, token, nil, context.Canceled)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled reachable", err)
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

func TestDoctorCommand_JSONPrintsEmptyArrayWheneverChecksCannotRun(t *testing.T) {
	for name, tc := range map[string]struct {
		setup    func(t *testing.T)
		token    string
		wantExit int
	}{
		"missing config": {setup: func(t *testing.T) { _ = os.Remove("okdctl.yaml") }, token: doctorToken, wantExit: 66},
		"invalid config": {setup: func(t *testing.T) {
			if err := os.WriteFile("okdctl.yaml", []byte("schemaVersion: v3\ncluster:\n  name: Bad_Name\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, token: doctorToken, wantExit: 2},
		"missing token": {setup: func(*testing.T) {}, token: "", wantExit: 5},
	} {
		t.Run(name, func(t *testing.T) {
			h := newDoctorHarness(t)
			tc.setup(t)
			out, err := h.run(t, tc.token, nil, nil, "-o", "json")
			if err == nil || exitCodeFor(err) != tc.wantExit || strings.TrimSpace(out) != "[]" {
				t.Fatalf("stdout = %q err = %v (exit %d), want [] and exit %d", out, err, exitCodeFor(err), tc.wantExit)
			}
			if h.seam != nil {
				t.Fatal("preflight ran")
			}
		})
	}
}

func TestDoctorCommand_InvalidConfigStopsBeforeCredentials(t *testing.T) {
	h := newDoctorHarness(t)
	if err := os.WriteFile("okdctl.yaml", []byte("schemaVersion: v3\ncluster:\n  name: Bad_Name\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := h.run(t, "", nil, nil)
	var ce *errtypes.ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %T %v, want *errtypes.ConfigError", err, err)
	}
	if h.seam != nil || !strings.Contains(h.errOut.String(), "cluster.name") {
		t.Fatalf("seam=%v stderr=%q; want validation findings and no preflight", h.seam, h.errOut.String())
	}
}
