package dns

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

	t.Run("missing_backup_removes_rejected_config", func(t *testing.T) {
		confPath, backupPath := setup(t, false)
		injectFns(t, errValidate, nil)

		err := validateAndRestartDnsmasq(context.Background(), clusterName)
		if err == nil {
			t.Fatal("expected non-nil error, got nil")
		}
		if !errors.Is(err, errValidate) {
			t.Errorf("errors.Is(err, errValidate) = false; got: %v", err)
		}
		if !strings.Contains(err.Error(), "rejected config removed") {
			t.Errorf("first-deploy rejection must report removal, got: %v", err)
		}

		// No backup ever existed, so the true previous state is absence: the
		// rejected drop-in this call wrote must not be left behind.
		if _, statErr := os.Stat(confPath); !os.IsNotExist(statErr) {
			t.Errorf("expected rejected config to be removed, stat err = %v", statErr)
		}
		if _, statErr := os.Stat(backupPath); !os.IsNotExist(statErr) {
			t.Errorf("expected backup to remain absent, stat err = %v", statErr)
		}
	})
}

// TestValidateAndRestartDnsmasq_RecoveryRestart covers the restart-failure
// branch: after restoring the backup the service is restarted once more so
// the running dnsmasq converges to the restored config, even when the caller
// ctx is already cancelled.
func TestValidateAndRestartDnsmasq_RecoveryRestart(t *testing.T) {
	errRestart := errors.New("restart failed")

	const clusterName = "okd-test"

	setup := func(t *testing.T) {
		t.Helper()
		dir := t.TempDir()
		origDir := dnsmasqConfigDir
		dnsmasqConfigDir = dir
		t.Cleanup(func() { dnsmasqConfigDir = origDir })
		confPath := filepath.Join(dir, clusterName+".conf")
		if err := os.WriteFile(confPath, []byte("live\n"), 0o644); err != nil {
			t.Fatalf("seed live config: %v", err)
		}
		if err := os.WriteFile(confPath+".backup", []byte("backup\n"), 0o644); err != nil {
			t.Fatalf("seed backup: %v", err)
		}
	}

	t.Run("recovery_restart_attempted_and_succeeds", func(t *testing.T) {
		setup(t)
		origV := validateDnsmasqConfigFn
		origR := restartDnsmasqFn
		t.Cleanup(func() {
			validateDnsmasqConfigFn = origV
			restartDnsmasqFn = origR
		})
		validateDnsmasqConfigFn = func(context.Context) error { return nil }

		calls := 0
		var recoveryCtxErr error
		restartDnsmasqFn = func(ctx context.Context) error {
			calls++
			if calls == 1 {
				return errRestart
			}
			recoveryCtxErr = ctx.Err()
			return nil
		}

		// Cancelled parent ctx: the recovery restart must still run.
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := validateAndRestartDnsmasq(ctx, clusterName)
		if err == nil {
			t.Fatal("expected non-nil error, got nil")
		}
		if !errors.Is(err, errRestart) {
			t.Errorf("errors.Is(err, errRestart) = false; got: %v", err)
		}
		if calls != 2 {
			t.Errorf("restart calls = %d; want 2 (failed restart + recovery restart)", calls)
		}
		if recoveryCtxErr != nil {
			t.Errorf("recovery restart ran under a cancelled ctx: %v", recoveryCtxErr)
		}
	})

	t.Run("recovery_restart_failure_reported", func(t *testing.T) {
		setup(t)
		origV := validateDnsmasqConfigFn
		origR := restartDnsmasqFn
		t.Cleanup(func() {
			validateDnsmasqConfigFn = origV
			restartDnsmasqFn = origR
		})
		validateDnsmasqConfigFn = func(context.Context) error { return nil }
		restartDnsmasqFn = func(context.Context) error { return errRestart }

		err := validateAndRestartDnsmasq(context.Background(), clusterName)
		if err == nil {
			t.Fatal("expected non-nil error, got nil")
		}
		if !strings.Contains(err.Error(), "also failed") {
			t.Errorf("error %q must report the failed recovery restart", err)
		}
	})
}
