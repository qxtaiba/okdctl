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
			if err := os.WriteFile(path, []byte("schemaVersion: v1\n"), tc.perm); err != nil {
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

func TestLoadFile_EmptySchemaVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "okdctl.yaml")
	// DefaultConfig() pre-populates SchemaVersion=v1, so YAML must explicitly
	// blank it to hit the empty-string branch.
	if err := os.WriteFile(path, []byte(`schemaVersion: ""`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := NewLoader().LoadFile(path)
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("err = %v; want *errtypes.ConfigError", err)
	}
	if !strings.Contains(cfgErr.Msg, SchemaVersionV1) {
		t.Errorf("ConfigError.Msg = %q; want it to contain %q", cfgErr.Msg, SchemaVersionV1)
	}
}

func TestLoadFile_UnsupportedSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "okdctl.yaml")
	if err := os.WriteFile(path, []byte("schemaVersion: v99\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := NewLoader().LoadFile(path)
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("err = %v; want *errtypes.ConfigError", err)
	}
}

func TestLoadFile_UnknownTopLevelKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "okdctl.yaml")
	if err := os.WriteFile(path, []byte("schemaVersion: v1\nunknownField: oops\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := NewLoader().LoadFile(path)
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("err = %v; want *errtypes.ConfigError wrapping UnmarshalStrict rejection", err)
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
	if loaded.SchemaVersion != SchemaVersionV1 {
		t.Errorf("SchemaVersion = %q; want %q", loaded.SchemaVersion, SchemaVersionV1)
	}
	if loaded.Cluster.Name != cfg.Cluster.Name {
		t.Errorf("Cluster.Name = %q; want %q", loaded.Cluster.Name, cfg.Cluster.Name)
	}
}
