package dns

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAndRestartDnsmasq(t *testing.T) {
	errValidate := errors.New("validate failed")
	errRestart := errors.New("restart failed")

	const (
		clusterName   = "okd-test"
		liveContent   = "address=/api.test.example.com/10.0.0.2\n"
		backupContent = "address=/api.test.example.com/10.0.0.1\n"
	)

	setup := func(t *testing.T, seedBackup bool) (confPath, backupPath string) {
		t.Helper()
		dir := t.TempDir()

		origDir := dnsmasqConfigDir
		dnsmasqConfigDir = dir
		t.Cleanup(func() { dnsmasqConfigDir = origDir })

		confPath = filepath.Join(dir, clusterName+".conf")
		backupPath = confPath + ".backup"

		if err := os.WriteFile(confPath, []byte(liveContent), 0o644); err != nil {
			t.Fatalf("seed live config: %v", err)
		}
		if seedBackup {
			if err := os.WriteFile(backupPath, []byte(backupContent), 0o644); err != nil {
				t.Fatalf("seed backup: %v", err)
			}
		}
		return confPath, backupPath
	}

	injectFns := func(t *testing.T, validateErr, restartErr error) {
		t.Helper()
		origV := validateDnsmasqConfigFn
		origR := restartDnsmasqFn
		validateDnsmasqConfigFn = func(_ context.Context) error { return validateErr }
		restartDnsmasqFn = func(_ context.Context) error { return restartErr }
		t.Cleanup(func() {
			validateDnsmasqConfigFn = origV
			restartDnsmasqFn = origR
		})
	}

	t.Run("happy_path_removes_backup", func(t *testing.T) {
		confPath, backupPath := setup(t, true)
		injectFns(t, nil, nil)

		err := validateAndRestartDnsmasq(context.Background(), clusterName)
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}

		got, err := os.ReadFile(confPath)
		if err != nil {
			t.Fatalf("read live config: %v", err)
		}
		if string(got) != liveContent {
			t.Errorf("live config content = %q; want %q", got, liveContent)
		}

		if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
			t.Errorf("expected backup to be removed, stat err = %v", err)
		}
	})

	t.Run("validate_failure_restores_backup", func(t *testing.T) {
		confPath, backupPath := setup(t, true)
		injectFns(t, errValidate, nil)

		err := validateAndRestartDnsmasq(context.Background(), clusterName)
		if err == nil {
			t.Fatal("expected non-nil error, got nil")
		}
		if !errors.Is(err, errValidate) {
			t.Errorf("errors.Is(err, errValidate) = false; got: %v", err)
		}

		got, readErr := os.ReadFile(confPath)
		if readErr != nil {
			t.Fatalf("read live config after restore: %v", readErr)
		}
		if string(got) != backupContent {
			t.Errorf("restored config content = %q; want %q", got, backupContent)
		}

		if _, statErr := os.Stat(backupPath); statErr != nil {
			t.Errorf("expected backup to still exist, stat err = %v", statErr)
		}
	})

	t.Run("restart_failure_restores_backup", func(t *testing.T) {
		confPath, backupPath := setup(t, true)
		injectFns(t, nil, errRestart)

		err := validateAndRestartDnsmasq(context.Background(), clusterName)
		if err == nil {
			t.Fatal("expected non-nil error, got nil")
		}
		if !errors.Is(err, errRestart) {
			t.Errorf("errors.Is(err, errRestart) = false; got: %v", err)
		}

		got, readErr := os.ReadFile(confPath)
		if readErr != nil {
			t.Fatalf("read live config after restore: %v", readErr)
		}
		if string(got) != backupContent {
			t.Errorf("restored config content = %q; want %q", got, backupContent)
		}

		if _, statErr := os.Stat(backupPath); statErr != nil {
			t.Errorf("expected backup to still exist, stat err = %v", statErr)
		}
	})

	t.Run("missing_backup_not_precondition", func(t *testing.T) {
		_, backupPath := setup(t, false)
		injectFns(t, errValidate, nil)

		err := validateAndRestartDnsmasq(context.Background(), clusterName)
		if err == nil {
			t.Fatal("expected non-nil error, got nil")
		}
		if !errors.Is(err, errValidate) {
			t.Errorf("errors.Is(err, errValidate) = false; got: %v", err)
		}

		if _, statErr := os.Stat(backupPath); !os.IsNotExist(statErr) {
			t.Errorf("expected backup to remain absent, stat err = %v", statErr)
		}
	})
}
