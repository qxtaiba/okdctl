package dns

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateConfigName(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid simple", input: "okd-prod"},
		{name: "valid alphanumeric", input: "mycluster1"},
		{name: "valid with underscore", input: "okd_dev"},
		{name: "max length 64", input: strings.Repeat("a", 64)},
		{name: "empty", input: "", wantErr: true},
		{name: "leading dot", input: ".hidden", wantErr: true},
		{name: "path traversal", input: "../etc/passwd", wantErr: true},
		{name: "slash", input: "etc/passwd", wantErr: true},
		{name: "dot inside", input: "okd.prod", wantErr: true},
		{name: "space", input: "okd prod", wantErr: true},
		{name: "special chars", input: "okd@prod!", wantErr: true},
		{name: "leading hyphen", input: "-leading", wantErr: true},
		{name: "too long", input: strings.Repeat("a", 65), wantErr: true},
		{name: "single char", input: "a"},
		{name: "two chars", input: "a1"},
		{name: "unicode", input: "é", wantErr: true},
		{name: "null byte", input: "a\x00b", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConfigName(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("validateConfigName(%q): expected error, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validateConfigName(%q): unexpected error: %v", tc.input, err)
			}
		})
	}
}

func TestDnsmasqConfigPath(t *testing.T) {
	t.Run("valid name returns expected path", func(t *testing.T) {
		got, err := DnsmasqConfigPath("okd-prod")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(dnsmasqConfigDir, "okd-prod.conf")
		if got != want {
			t.Errorf("DnsmasqConfigPath = %q; want %q", got, want)
		}
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		_, err := DnsmasqConfigPath("../etc/passwd")
		if err == nil {
			t.Error("expected error for path traversal input, got nil")
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		_, err := DnsmasqConfigPath("")
		if err == nil {
			t.Error("expected error for empty name, got nil")
		}
	})
}

func TestConfigName(t *testing.T) {
	got := configName("prod")
	if got != "okd-prod" {
		t.Errorf("configName(%q) = %q; want %q", "prod", got, "okd-prod")
	}
}

// TestWriteDnsmasqConfig locks the backup-then-write contract that
// validateAndRestartDnsmasq's tested rollback depends on: the .backup file
// it restores from is created only here.
func TestWriteDnsmasqConfig(t *testing.T) {
	redirect := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		orig := dnsmasqConfigDir
		dnsmasqConfigDir = dir
		t.Cleanup(func() { dnsmasqConfigDir = orig })
		return dir
	}

	t.Run("first write creates conf without backup", func(t *testing.T) {
		dir := redirect(t)
		if err := writeDnsmasqConfig(context.Background(), "okd", "address=/api/10.0.0.1\n"); err != nil {
			t.Fatalf("writeDnsmasqConfig: %v", err)
		}
		confPath := filepath.Join(dir, "okd.conf")
		fi, err := os.Stat(confPath)
		if err != nil {
			t.Fatalf("conf not written: %v", err)
		}
		if perm := fi.Mode().Perm(); perm != 0o644 {
			t.Errorf("conf perm = %#o, want 0644", perm)
		}
		if _, err := os.Stat(confPath + ".backup"); !os.IsNotExist(err) {
			t.Errorf("first write must not create a backup, stat err: %v", err)
		}
	})

	t.Run("overwrite backs up prior bytes before replacing", func(t *testing.T) {
		dir := redirect(t)
		confPath := filepath.Join(dir, "okd.conf")
		prior := "address=/api/10.0.0.1\n"
		if err := os.WriteFile(confPath, []byte(prior), 0o644); err != nil {
			t.Fatal(err)
		}

		next := "address=/api/10.0.0.2\n"
		if err := writeDnsmasqConfig(context.Background(), "okd", next); err != nil {
			t.Fatalf("writeDnsmasqConfig: %v", err)
		}

		backup, err := os.ReadFile(confPath + ".backup")
		if err != nil {
			t.Fatalf("overwrite must create %s.backup: %v", confPath, err)
		}
		if string(backup) != prior {
			t.Errorf("backup = %q, want the pre-overwrite bytes %q", backup, prior)
		}
		live, err := os.ReadFile(confPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(live) != next {
			t.Errorf("live conf = %q, want %q", live, next)
		}
	})

	t.Run("invalid name rejected before any file op", func(t *testing.T) {
		dir := redirect(t)
		err := writeDnsmasqConfig(context.Background(), "../evil", "x")
		if err == nil {
			t.Fatal("path-traversal name must be rejected")
		}
		entries, readErr := os.ReadDir(dir)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(entries) != 0 {
			t.Errorf("rejected write must not touch the config dir, found %d entries", len(entries))
		}
	})

	t.Run("cancelled ctx writes nothing", func(t *testing.T) {
		dir := redirect(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := writeDnsmasqConfig(ctx, "okd", "x"); !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got: %v", err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("cancelled write must not touch the config dir, found %d entries", len(entries))
		}
	})
}
