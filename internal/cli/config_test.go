package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func runValidate(t *testing.T, configFile, output string) (stdout, stderr string, err error) {
	t.Helper()
	origCfg := cfgFile
	cfgFile = configFile
	t.Cleanup(func() {
		cfgFile = origCfg
		_ = configValidateCmd.Flags().Set(flagOutput, "text")
	})
	if output != "" {
		if err := configValidateCmd.Flags().Set(flagOutput, output); err != nil {
			t.Fatalf("set --output: %v", err)
		}
	}
	var out, errOut bytes.Buffer
	configValidateCmd.SetOut(&out)
	configValidateCmd.SetErr(&errOut)
	t.Cleanup(func() {
		configValidateCmd.SetOut(nil)
		configValidateCmd.SetErr(nil)
	})
	err = runConfigValidate(configValidateCmd, nil)
	return out.String(), errOut.String(), err
}

func standardExample(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "configs", "examples", "standard.yaml")
}

func TestConfigValidate_TextPrintsNodeTable(t *testing.T) {
	stdout, _, err := runValidate(t, standardExample(t), "")
	if err != nil {
		t.Fatalf("runConfigValidate: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "configuration is valid") {
		t.Errorf("summary missing:\n%s", stdout)
	}
	lines := strings.Split(stdout, "\n")
	header := -1
	for i, line := range lines {
		if strings.Fields(line) != nil && strings.Join(strings.Fields(line), " ") == "NAME ROLE VMID MAC IP HOST CPU MEM DISKS" {
			header = i
		}
	}
	if header < 0 {
		t.Fatalf("header line missing:\n%s", stdout)
	}
	rows := lines[header+1:]
	if len(rows) < 5 {
		t.Fatalf("want 5 node rows after the header, got %d:\n%s", len(rows), stdout)
	}
	master0 := strings.Fields(rows[0])
	want := []string{"standard-master0", "master", "6010", "02:", "192.168.1.150", "pve1", "4", "32768", "120"}
	if len(master0) != len(want) {
		t.Fatalf("row = %q; want %d columns", rows[0], len(want))
	}
	for i := range want {
		if !strings.HasPrefix(master0[i], want[i]) {
			t.Errorf("row[%d] = %q; want prefix %q (row %q)", i, master0[i], want[i], rows[0])
		}
	}
	worker0 := strings.Fields(rows[3])
	if worker0[0] != "standard-worker0" || worker0[8] != "120,500" || worker0[4] != "192.168.1.160" {
		t.Errorf("worker row = %q", rows[3])
	}
}

func TestConfigValidate_JSONCarriesInventoryOnly(t *testing.T) {
	stdout, stderr, err := runValidate(t, standardExample(t), "json")
	if err != nil {
		t.Fatalf("runConfigValidate: %v", err)
	}
	var inv struct {
		InfraID    string `json:"infra_id"`
		APIVIP     string `json:"api_vip"`
		IngressVIP string `json:"ingress_vip"`
		Nodes      []struct {
			Name  string `json:"name"`
			Role  string `json:"role"`
			VMID  int    `json:"vmid"`
			MAC   string `json:"mac"`
			IP    string `json:"ip"`
			Host  string `json:"host"`
			Disks []struct {
				SizeGiB int `json:"size_gib"`
			} `json:"disks"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(stdout), &inv); err != nil {
		t.Fatalf("stdout is not inventory JSON: %v\n%s", err, stdout)
	}
	if inv.InfraID != "preview" || inv.APIVIP != "192.168.1.158" || inv.IngressVIP != "192.168.1.159" || len(inv.Nodes) != 5 {
		t.Errorf("inventory = %+v", inv)
	}
	if n := inv.Nodes[4]; n.Name != "standard-worker1" || n.Role != "worker" || n.VMID != 6101 || n.IP != "192.168.1.161" || len(n.Disks) != 2 {
		t.Errorf("worker1 = %+v", n)
	}
	if !strings.Contains(stderr, "configuration is valid") {
		t.Errorf("summary must go to stderr in json mode; stderr = %q", stderr)
	}
}

func TestConfigValidate_UnknownOutputIsUsageError(t *testing.T) {
	_, _, err := runValidate(t, standardExample(t), "yaml")
	var ue *errtypes.UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("err = %v; want *errtypes.UsageError", err)
	}
}

func TestConfigValidate_InvalidPrintsNoTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "okdctl.yaml")
	if err := os.WriteFile(path, []byte("schemaVersion: v3\ncluster:\n  name: Bad_Name\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{"", "json"} {
		t.Run("output="+output, func(t *testing.T) {
			stdout, stderr, err := runValidate(t, path, output)
			var cfgErr *errtypes.ConfigError
			if !errors.As(err, &cfgErr) {
				t.Fatalf("err = %v; want *errtypes.ConfigError", err)
			}
			if strings.Contains(stdout, "NAME") || strings.Contains(stdout, `"nodes"`) {
				t.Errorf("table printed for an invalid config:\n%s", stdout)
			}
			summary := stdout
			if output == "json" {
				summary = stderr
				if stdout != "" {
					t.Errorf("json stdout must be empty on failure; got %q", stdout)
				}
			}
			if !strings.Contains(summary, "configuration invalid") {
				t.Errorf("summary missing; stdout=%q stderr=%q", stdout, stderr)
			}
		})
	}
}
