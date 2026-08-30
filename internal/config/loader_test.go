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
		{"unsupported schemaVersion", "schemaVersion: v99\n", ""},
		{"unknown top-level key", "schemaVersion: v2\nunknownField: oops\n", ""},
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
			if tc.wantInMsg != "" && !strings.Contains(cfgErr.Msg, tc.wantInMsg) {
				t.Errorf("ConfigError.Msg = %q; want it to contain %q", cfgErr.Msg, tc.wantInMsg)
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
