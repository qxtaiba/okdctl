package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

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
			dir := t.TempDir()
			path := filepath.Join(dir, "okdctl.yaml")
			if err := os.WriteFile(path, []byte("schemaVersion: v2\n"), tc.perm); err != nil {
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

func TestLoadFile_Rejections(t *testing.T) {
	cases := []struct {
		name      string
		yaml      string
		wantInMsg string
	}{
		{"schemaVersion explicitly empty", `schemaVersion: ""` + "\n", SchemaVersionCurrent},
		{"schemaVersion absent", "cluster:\n  name: mycluster\n", SchemaVersionCurrent},
		{"unsupported schemaVersion", "schemaVersion: v99\n", `unsupported schemaVersion "v99" (expected "v2")`},
		{"unknown top-level key", "schemaVersion: v2\nunknownField: oops\n", `unknown field "unknownField"`},
		{"unknown nested key", "schemaVersion: v2\ncluster:\n  nickname: oops\n", `unknown field "nickname"`},
		{"wrong value type", "schemaVersion: v2\ntopology:\n  workers:\n    count: three\n", "topology.workers.count"},
		{"duplicate key", "schemaVersion: v2\ncluster:\n  name: a\n  name: b\n", `key "name" already set`},
		{"syntax error", "schemaVersion: v2\ncluster:\n  name: [unterminated\n", "line 3: did not find expected"},
		{"second document", "schemaVersion: v2\n---\ncluster:\n  name: second\n", "more than one YAML document"},
		{"leading marker then second document", "---\nschemaVersion: v2\n---\ncluster:\n  name: second\n", "more than one YAML document"},
		{"empty trailing document", "schemaVersion: v2\n---\n", "more than one YAML document"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "okdctl.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o600); err != nil {
				t.Fatal(err)
			}

			_, err := NewLoader().LoadFile(path)
			var cfgErr *errtypes.ConfigError
			if !errors.As(err, &cfgErr) {
				t.Fatalf("err = %v; want *errtypes.ConfigError", err)
			}
			if !strings.Contains(cfgErr.Msg, tc.wantInMsg) {
				t.Errorf("ConfigError.Msg = %q; want it to contain %q", cfgErr.Msg, tc.wantInMsg)
			}
			if !strings.Contains(cfgErr.Msg, path) {
				t.Errorf("ConfigError.Msg = %q; want it to name %s", cfgErr.Msg, path)
			}
		})
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
				"PROXMOX_VE_USERNAME=root@pam\n" +
				"PROXMOX_VE_PASSWORD=" + secret + "\n",
		},
		{"token assignment", "PROXMOX_VE_API_TOKEN=root@pam!okdctl=" + secret + "\n"},
		{"value that parses as a yaml alias", "PROXMOX_VE_PASSWORD=ab: *" + secret + "\n"},
		{"value that parses as a tagged scalar", "PROXMOX_VE_PASSWORD=ab: !!int " + secret + "\n"},
		{"alias value ahead of other assignments", "PROXMOX_VE_PASSWORD=ab: *" + secret + "\nPROXMOX_VE_USERNAME=root@pam\n"},
		{"indented assignment", "  PROXMOX_VE_PASSWORD=ab: *" + secret + "\n"},
		{"assignment beside a schemaVersion", "schemaVersion: v2\nPROXMOX_VE_PASSWORD=" + secret + ": tail\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "okdctl.env")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
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
	path := filepath.Join(t.TempDir(), "okdctl.yaml")
	if err := os.WriteFile(path, []byte("schemaVersion: v2\nPROXMOX_VE_PASSWORD: "+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewLoader().LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), `unknown field "PROXMOX_VE_PASSWORD"`) {
		t.Fatalf("err = %v; want the unknown key named", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("error echoes the value: %q", err.Error())
	}
}

func TestLoadFile_AcceptsLeadingDocumentMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "okdctl.yaml")
	if err := os.WriteFile(path, []byte("---\nschemaVersion: v2\ncluster:\n  name: marked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := NewLoader().LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if cfg.Cluster.Name != "marked" {
		t.Errorf("Cluster.Name = %q; want %q", cfg.Cluster.Name, "marked")
	}
}

func TestExampleConfigs_LoadValidateAndKeepClusterName(t *testing.T) {
	wantName := map[string]string{
		"media-server.yaml": "grappleberry",
		"minimal.yaml":      "minimal",
		"production.yaml":   "production",
	}
	paths, err := filepath.Glob(filepath.Join("..", "..", "configs", "examples", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != len(wantName) {
		t.Fatalf("found %d example configs %v; want %d", len(paths), paths, len(wantName))
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			want, ok := wantName[filepath.Base(path)]
			if !ok {
				t.Fatalf("no expected cluster.name recorded for %s", path)
			}
			cfg, err := NewLoader().LoadFile(path)
			if err != nil {
				t.Fatalf("LoadFile: %v", err)
			}
			if cfg.Cluster.Name != want {
				t.Errorf("Cluster.Name = %q; want %q", cfg.Cluster.Name, want)
			}
			if result := cfg.Validate(); !result.IsValid() {
				t.Errorf("Validate: %v", result)
			}
		})
	}
}

func TestLoadFile_DerivesNetmaskFromMachineCIDR(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "stale netmask overwritten",
			yaml: "schemaVersion: v2\nnetworking:\n  machine_cidr: 192.168.2.0/25\n  static_ip:\n    netmask: 255.255.255.0\n",
			want: "255.255.255.128",
		},
		{
			name: "absent netmask derived",
			yaml: "schemaVersion: v2\nnetworking:\n  machine_cidr: 10.0.0.0/16\n",
			want: "255.255.0.0",
		},
		{
			name: "invalid cidr leaves netmask for validators",
			yaml: "schemaVersion: v2\nnetworking:\n  machine_cidr: not-a-cidr\n  static_ip:\n    netmask: 255.255.0.0\n",
			want: "255.255.0.0",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "okdctl.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := NewLoader().LoadFile(path)
			if err != nil {
				t.Fatalf("LoadFile: %v", err)
			}
			if got := cfg.Networking.StaticIP.Netmask; got != tc.want {
				t.Errorf("Netmask = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestLoadFile_SaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "okdctl.yaml")

	loader := NewLoader()
	cfg := DefaultConfig()
	if err := loader.Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("Save perm = %#o; want 0o600", perm)
	}

	loaded, err := loader.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile after Save: %v", err)
	}
	if loaded.SchemaVersion != SchemaVersionCurrent {
		t.Errorf("SchemaVersion = %q; want %q", loaded.SchemaVersion, SchemaVersionCurrent)
	}
	if loaded.Cluster.Name != cfg.Cluster.Name {
		t.Errorf("Cluster.Name = %q; want %q", loaded.Cluster.Name, cfg.Cluster.Name)
	}
}
