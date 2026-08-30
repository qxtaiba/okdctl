package dns

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubDnsmasqFns swaps the validate/restart test seams for the test's duration.
func stubDnsmasqFns(t *testing.T, validate, restart func(context.Context) error) {
	t.Helper()
	origV, origR := validateDnsmasqConfigFn, restartDnsmasqFn
	validateDnsmasqConfigFn = validate
	restartDnsmasqFn = restart
	t.Cleanup(func() {
		validateDnsmasqConfigFn = origV
		restartDnsmasqFn = origR
	})
}

const destructiveClusterName = "okd-test"

// seedDnsmasqConf redirects the config dir and seeds a live drop-in (plus .backup when non-empty).
func seedDnsmasqConf(t *testing.T, live, backup string) (confPath, backupPath string) {
	t.Helper()
	dir := redirectConfigDir(t)
	confPath = filepath.Join(dir, destructiveClusterName+".conf")
	backupPath = confPath + ".backup"
	if err := os.WriteFile(confPath, []byte(live), 0o644); err != nil {
		t.Fatalf("seed live config: %v", err)
	}
	if backup != "" {
		if err := os.WriteFile(backupPath, []byte(backup), 0o644); err != nil {
			t.Fatalf("seed backup: %v", err)
		}
	}
	return confPath, backupPath
}

func TestValidateAndRestartDnsmasq(t *testing.T) {
	errValidate := errors.New("validate failed")
	errRestart := errors.New("restart failed")

	const (
		liveContent   = "address=/api.test.example.com/10.0.0.2\n"
		backupContent = "address=/api.test.example.com/10.0.0.1\n"
	)

	injectFns := func(t *testing.T, validateErr, restartErr error) {
		stubDnsmasqFns(t,
			func(context.Context) error { return validateErr },
			func(context.Context) error { return restartErr })
	}

	t.Run("happy_path_removes_backup", func(t *testing.T) {
		confPath, backupPath := seedDnsmasqConf(t, liveContent, backupContent)
		injectFns(t, nil, nil)

		err := validateAndRestartDnsmasq(context.Background(), destructiveClusterName)
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

	restoreCases := []struct {
		name        string
		validateErr error
		restartErr  error
	}{
		{"validate_failure_restores_backup", errValidate, nil},
		{"restart_failure_restores_backup", nil, errRestart},
	}
	for _, tc := range restoreCases {
		t.Run(tc.name, func(t *testing.T) {
			confPath, backupPath := seedDnsmasqConf(t, liveContent, backupContent)
			injectFns(t, tc.validateErr, tc.restartErr)

			err := validateAndRestartDnsmasq(context.Background(), destructiveClusterName)
			if err == nil {
				t.Fatal("expected non-nil error, got nil")
			}
			wantErr := tc.validateErr
			if wantErr == nil {
				wantErr = tc.restartErr
			}
			if !errors.Is(err, wantErr) {
				t.Errorf("errors.Is(err, %v) = false; got: %v", wantErr, err)
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
	}

	t.Run("missing_backup_removes_rejected_config", func(t *testing.T) {
		confPath, backupPath := seedDnsmasqConf(t, liveContent, "")
		injectFns(t, errValidate, nil)

		err := validateAndRestartDnsmasq(context.Background(), destructiveClusterName)
		if err == nil {
			t.Fatal("expected non-nil error, got nil")
		}
		if !errors.Is(err, errValidate) {
			t.Errorf("errors.Is(err, errValidate) = false; got: %v", err)
		}
		if !strings.Contains(err.Error(), "rejected config removed") {
			t.Errorf("first-deploy rejection must report removal, got: %v", err)
		}

		// No backup ever existed; the rejected drop-in this call wrote must not remain.
		if _, statErr := os.Stat(confPath); !os.IsNotExist(statErr) {
			t.Errorf("expected rejected config to be removed, stat err = %v", statErr)
		}
		if _, statErr := os.Stat(backupPath); !os.IsNotExist(statErr) {
			t.Errorf("expected backup to remain absent, stat err = %v", statErr)
		}
	})
}

// TestValidateAndRestartDnsmasq_RecoveryRestart exercises the recovery restart
// even when the caller ctx is already cancelled.
func TestValidateAndRestartDnsmasq_RecoveryRestart(t *testing.T) {
	errRestart := errors.New("restart failed")

	t.Run("recovery_restart_attempted_and_succeeds", func(t *testing.T) {
		seedDnsmasqConf(t, "live\n", "backup\n")

		calls := 0
		var recoveryCtxErr error
		stubDnsmasqFns(t,
			func(context.Context) error { return nil },
			func(ctx context.Context) error {
				calls++
				if calls == 1 {
					return errRestart
				}
				recoveryCtxErr = ctx.Err()
				return nil
			})

		// Cancelled parent ctx: the recovery restart must still run.
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := validateAndRestartDnsmasq(ctx, destructiveClusterName)
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
		seedDnsmasqConf(t, "live\n", "backup\n")
		stubDnsmasqFns(t,
			func(context.Context) error { return nil },
			func(context.Context) error { return errRestart })

		err := validateAndRestartDnsmasq(context.Background(), destructiveClusterName)
		if err == nil {
			t.Fatal("expected non-nil error, got nil")
		}
		if !strings.Contains(err.Error(), "also failed") {
			t.Errorf("error %q must report the failed recovery restart", err)
		}
	})
}
