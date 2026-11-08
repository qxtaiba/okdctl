package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

const v3Header = "schemaVersion: v3\n"

func writeConfig(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFile_PermGate(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses 0o022 permission gate")
	}
	tests := []struct {
		name        string
		perm        os.FileMode
		wantAuthErr bool
	}{
		{"0600 accepted", 0o600, false},
		{"0400 accepted", 0o400, false},
		{"0620 group-writable rejected", 0o620, true},
		{"0602 world-writable rejected", 0o602, true},
		{"0666 group+world-writable rejected", 0o666, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "okdctl.yaml")
			if err := os.WriteFile(path, []byte(v3Header), tc.perm); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, tc.perm); err != nil {
				t.Fatal(err)
			}

			_, err := NewLoader().LoadFile(path)
			var authErr *errtypes.AuthError
			gotAuth := errors.As(err, &authErr)
			if gotAuth != tc.wantAuthErr {
				t.Errorf("err = %v, wantAuthErr = %v", err, tc.wantAuthErr)
			}
		})
	}
}

func TestLoadFile_RefusesSchemaV2(t *testing.T) {
	path := writeConfig(t, "okdctl.yaml", "schemaVersion: v2\ncluster:\n  name: legacy\n")

	_, err := NewLoader().LoadFile(path)

	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("err = %v; want *errtypes.ConfigError", err)
	}
	const want = "okdctl.yaml is schema v2; this release reads v3 only, see docs/migration.md"
	if cfgErr.Msg != want {
		t.Errorf("ConfigError.Msg = %q; want %q", cfgErr.Msg, want)
	}
	display, ok := errtypes.Describe(err)
	if !ok || display.Kind != errtypes.KindConfig {
		t.Errorf("Describe = %+v, %v; want KindConfig", display, ok)
	}
	if got := display.Kind.ExitCode(); got != 2 {
		t.Errorf("ExitCode = %d; want 2", got)
	}
}

func TestLoadFile_Rejections(t *testing.T) {
	cases := []struct {
		name      string
		yaml      string
		wantInMsg string
	}{
		{"schemaVersion explicitly empty", `schemaVersion: ""` + "\n", "has no schemaVersion; this release reads v3 only"},
		{"schemaVersion absent", "cluster:\n  name: mycluster\n", "has no schemaVersion; this release reads v3 only"},
		{"unsupported schemaVersion", "schemaVersion: v99\n", "is schema v99; this release reads v3 only, see docs/migration.md"},
		{"unknown top-level key", v3Header + "unknownField: oops\n", `unknown field "unknownField"`},
		{"unknown nested key", v3Header + "cluster:\n  nickname: oops\n", `unknown field "nickname"`},
		{"removed v2 key", v3Header + "topology:\n  workers:\n    count: 2\n", `unknown field "topology"`},
		{"wrong value type", v3Header + "workers:\n  count: three\n", "workers.count"},
		{"malformed duration", v3Header + "lifecycle:\n  drain_timeout: soon\n", "lifecycle.drain_timeout"},
		{"duplicate key", v3Header + "cluster:\n  name: a\n  name: b\n", `key "name" already set`},
		{"syntax error", v3Header + "cluster:\n  name: [unterminated\n", "line 3: did not find expected"},
		{"second document", v3Header + "---\ncluster:\n  name: second\n", "more than one YAML document"},
		{"leading marker then second document", "---\n" + v3Header + "---\ncluster:\n  name: second\n", "more than one YAML document"},
		{"empty trailing document", v3Header + "---\n", "more than one YAML document"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfig(t, "okdctl.yaml", tc.yaml)

			_, err := NewLoader().LoadFile(path)
			var cfgErr *errtypes.ConfigError
			if !errors.As(err, &cfgErr) {
				t.Fatalf("err = %v; want *errtypes.ConfigError", err)
			}
			if !strings.Contains(cfgErr.Msg, tc.wantInMsg) {
				t.Errorf("ConfigError.Msg = %q; want it to contain %q", cfgErr.Msg, tc.wantInMsg)
			}
			if !strings.Contains(cfgErr.Msg, filepath.Base(path)) {
				t.Errorf("ConfigError.Msg = %q; want it to name %s", cfgErr.Msg, filepath.Base(path))
			}
		})
	}
}

func TestLoadFile_ZeroBaseLeavesMissingClusterNameEmpty(t *testing.T) {
	path := writeConfig(t, "okdctl.yaml", v3Header+"cluster:\n  domain: k8s.local\n")

	cfg, err := NewLoader().LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if cfg.Cluster.Name != "" {
		t.Errorf("Cluster.Name = %q; want it left empty, not defaulted", cfg.Cluster.Name)
	}
	result := cfg.Validate()
	if result.IsValid() {
		t.Fatal("Validate accepted a config without cluster.name")
	}
	found := false
	for _, e := range result.Errors {
		if e.Field == FieldClusterName {
			found = true
		}
	}
	if !found {
		t.Errorf("Validate errors = %v; want one on %s", result.Errors, FieldClusterName)
	}
}

func loadV3Fixture(t *testing.T) *Config {
	t.Helper()
	path := writeConfig(t, "okdctl.yaml", v3Header+`cluster: {name: lab, domain: k8s.local}
provider:
  type: proxmox
  proxmox:
    endpoint: https://pve.example:8006
    nodes: [pve1, pve2]
    vlan: 12
    ha: true
    vmid_base: 6000
networking:
  node_ip_start: 192.168.1.150
  ntp: [192.168.1.5]
  host_prefix: 23
control_plane:
  cpu: 4
  memory_mib: 16384
  disks: [{size_gib: 120}]
workers:
  count: 2
  cpu: 8
  memory_mib: 16384
  disks: [{size_gib: 120}, {size_gib: 500, storage: local-lvm, wipe: true}]
nodes:
  lab-master0: {memory_mib: 32768}
  lab-worker3: {host: pve2, ip: 192.168.1.170}
lifecycle:
  before_next_node: {command: "./hook.sh", timeout: 15m}
  drain_timeout: 10m
handoff: {enabled: true}
defaults: {disable_redhat_catalogs: true}
`)

	cfg, err := NewLoader().LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	return cfg
}

func TestLoadFile_ParsesV3Provisioning(t *testing.T) {
	cfg := loadV3Fixture(t)
	px := cfg.Provider.Proxmox
	if px.Endpoint != "https://pve.example:8006" || len(px.Nodes) != 2 || px.VLAN != 12 || !px.HA || px.VMIDBase != 6000 {
		t.Errorf("Proxmox = %+v", px)
	}
	if cfg.Networking.NodeIPStart != "192.168.1.150" || len(cfg.Networking.NTP) != 1 || cfg.Networking.HostPrefix != 23 {
		t.Errorf("Networking = %+v", cfg.Networking)
	}
	if cfg.ControlPlane.CPU != 4 || cfg.ControlPlane.MemoryMiB != 16384 || len(cfg.ControlPlane.Disks) != 1 || cfg.ControlPlane.Disks[0].SizeGiB != 120 {
		t.Errorf("ControlPlane = %+v", cfg.ControlPlane)
	}
	if cfg.Workers.Count != 2 || cfg.Workers.CPU != 8 || len(cfg.Workers.Disks) != 2 {
		t.Fatalf("Workers = %+v", cfg.Workers)
	}
	data := cfg.Workers.Disks[1]
	if data.SizeGiB != 500 || data.Storage != "local-lvm" || data.Wipe == nil || !*data.Wipe {
		t.Errorf("Workers.Disks[1] = %+v", data)
	}
	if cfg.Workers.Disks[0].Wipe != nil {
		t.Errorf("Workers.Disks[0].Wipe = %v; want nil when unset", *cfg.Workers.Disks[0].Wipe)
	}
}

func TestLoadFile_ParsesV3NodesAndLifecycle(t *testing.T) {
	cfg := loadV3Fixture(t)
	master := cfg.Nodes["lab-master0"]
	if master.MemoryMiB == nil || *master.MemoryMiB != 32768 || master.CPU != nil {
		t.Errorf("Nodes[lab-master0] = %+v", master)
	}
	worker := cfg.Nodes["lab-worker3"]
	if worker.Host != "pve2" || worker.IP != "192.168.1.170" || worker.MemoryMiB != nil {
		t.Errorf("Nodes[lab-worker3] = %+v", worker)
	}
	if cfg.Lifecycle.BeforeNextNode.Command != "./hook.sh" || cfg.Lifecycle.BeforeNextNode.Timeout != 15*time.Minute {
		t.Errorf("BeforeNextNode = %+v", cfg.Lifecycle.BeforeNextNode)
	}
	if cfg.Lifecycle.DrainTimeout != 10*time.Minute {
		t.Errorf("DrainTimeout = %v; want 10m", cfg.Lifecycle.DrainTimeout)
	}
	if !cfg.Handoff.Enabled || !cfg.Defaults.DisableRedHatCatalogs {
		t.Errorf("Handoff = %+v, Defaults = %+v", cfg.Handoff, cfg.Defaults)
	}
}

func TestLoadFile_FilesystemFailureCarriesCause(t *testing.T) {
	dir := t.TempDir()
	notADir := filepath.Join(dir, "plain-file")
	if err := os.WriteFile(notADir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		path      string
		wantInMsg string
	}{
		{"stat under a regular file", filepath.Join(notADir, "okdctl.yaml"), "not a directory"},
		{"read a directory", dir, "is a directory"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewLoader().LoadFile(tc.path)
			var cfgErr *errtypes.ConfigError
			if !errors.As(err, &cfgErr) {
				t.Fatalf("err = %v; want *errtypes.ConfigError", err)
			}
			if !strings.Contains(cfgErr.Msg, tc.wantInMsg) || !strings.Contains(cfgErr.Msg, tc.path) {
				t.Errorf("ConfigError.Msg = %q; want it to contain %q and %s", cfgErr.Msg, tc.wantInMsg, tc.path)
			}
		})
	}
}

func TestLoadFile_RefusesEnvFileWithoutEchoingItsValues(t *testing.T) {
	const secret = "hunter2s3cr3t"
	cases := []struct {
		name string
		body string
	}{
		{
			"env file as okdctl writes it",
			"# Proxmox credentials (managed by okdctl)\n" +
				"PROXMOX_VE_ENDPOINT=https://192.168.1.100:8006\n" +
				"PROXMOX_VE_API_TOKEN=root@pam!okdctl=" + secret + "\n" +
				"PROXMOX_VE_INSECURE=false\n",
		},
		{"token assignment", "PROXMOX_VE_API_TOKEN=root@pam!okdctl=" + secret + "\n"},
		{"value that parses as a yaml alias", "PROXMOX_VE_API_TOKEN=ab: *" + secret + "\n"},
		{"value that parses as a tagged scalar", "PROXMOX_VE_API_TOKEN=ab: !!int " + secret + "\n"},
		{"alias value ahead of other assignments", "PROXMOX_VE_API_TOKEN=ab: *" + secret + "\nPROXMOX_VE_INSECURE=false\n"},
		{"indented assignment", "  PROXMOX_VE_API_TOKEN=ab: *" + secret + "\n"},
		{"assignment beside a schemaVersion", v3Header + "PROXMOX_VE_API_TOKEN=" + secret + ": tail\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfig(t, "okdctl.env", tc.body)
			_, err := NewLoader().LoadFile(path)
			var cfgErr *errtypes.ConfigError
			if !errors.As(err, &cfgErr) {
				t.Fatalf("err = %v; want *errtypes.ConfigError", err)
			}
			if !strings.Contains(cfgErr.Msg, "credentials env file") || !strings.Contains(cfgErr.Msg, path) {
				t.Errorf("ConfigError.Msg = %q; want it to call %s a credentials env file", cfgErr.Msg, path)
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("error echoes the env value: %q", err.Error())
			}
		})
	}
}

func TestLoadFile_UnknownSecretKeyIsNamedWithoutItsValue(t *testing.T) {
	const secret = "hunter2s3cr3t"
	path := writeConfig(t, "okdctl.yaml", v3Header+"PROXMOX_VE_API_TOKEN: "+secret+"\n")
	_, err := NewLoader().LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), `unknown field "PROXMOX_VE_API_TOKEN"`) {
		t.Fatalf("err = %v; want the unknown key named", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("error echoes the value: %q", err.Error())
	}
}

func TestLoadFile_AcceptsLeadingDocumentMarker(t *testing.T) {
	path := writeConfig(t, "okdctl.yaml", "---\n"+v3Header+"cluster:\n  name: marked\n")
	cfg, err := NewLoader().LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if cfg.Cluster.Name != "marked" {
		t.Errorf("Cluster.Name = %q; want %q", cfg.Cluster.Name, "marked")
	}
}

func TestExampleConfigs_LoadAndValidate(t *testing.T) {
	want := map[string]struct {
		name        string
		workers     int
		overrides   int
		workerDisks int
	}{
		"minimal.yaml":  {"minimal", 0, 0, 1},
		"standard.yaml": {"standard", 2, 1, 2},
	}
	paths, err := filepath.Glob(filepath.Join("..", "..", "configs", "examples", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != len(want) {
		t.Fatalf("found %d example configs %v; want %d", len(paths), paths, len(want))
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			w, ok := want[filepath.Base(path)]
			if !ok {
				t.Fatalf("no expectation recorded for %s", path)
			}
			cfg, err := NewLoader().LoadFile(path)
			if err != nil {
				t.Fatalf("LoadFile: %v", err)
			}
			if cfg.Cluster.Name != w.name {
				t.Errorf("Cluster.Name = %q; want %q", cfg.Cluster.Name, w.name)
			}
			if cfg.Workers.Count != w.workers || len(cfg.Nodes) != w.overrides || len(cfg.Workers.Disks) != w.workerDisks {
				t.Errorf("workers = %d, overrides = %d, worker disks = %d; want %d, %d, %d",
					cfg.Workers.Count, len(cfg.Nodes), len(cfg.Workers.Disks), w.workers, w.overrides, w.workerDisks)
			}
			if w.workerDisks == 2 && cfg.Workers.Disks[1].SizeGiB != 500 {
				t.Errorf("Workers.Disks[1].SizeGiB = %d; want 500", cfg.Workers.Disks[1].SizeGiB)
			}
			if result := cfg.Validate(); !result.IsValid() {
				t.Errorf("Validate: %v", result)
			}
		})
	}
}
