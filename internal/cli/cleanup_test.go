package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

func cleanupGuardConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = guardTestCluster
	return cfg
}

func TestCleanupKindRemovesCredentials(t *testing.T) {
	removing := []cleanup.Kind{cleanup.Full, cleanup.WorkOnly}
	for _, k := range removing {
		if !cleanupKindRemovesCredentials(k) {
			t.Errorf("kind %q wipes cluster-config; must be classified credential-removing", k)
		}
	}
	scoped := []cleanup.Kind{cleanup.WebOnly, cleanup.HAProxyOnly, cleanup.TerraformOnly}
	for _, k := range scoped {
		if cleanupKindRemovesCredentials(k) {
			t.Errorf("kind %q never touches cluster-config; must not demand the typed-name gate", k)
		}
	}
}

// TestConfirmCleanupInteractive drives the real gate: credential-removing
// kinds (full, work-only) demand the exact cluster name before the y/N —
// matching destroy's two-stage gate — while scoped kinds keep the single
// y/N prompt.
func TestConfirmCleanupInteractive(t *testing.T) {
	cases := []struct {
		name    string
		kind    cleanup.Kind
		input   []string
		proceed bool
	}{
		{"full: wrong cluster name refuses", cleanup.Full, []string{"prod-oops\n"}, false},
		{"full: bare y at name stage refuses", cleanup.Full, []string{"y\n"}, false},
		{"full: exact name then yes proceeds", cleanup.Full, []string{"prod\n", "y\n"}, true},
		{"full: exact name then no aborts", cleanup.Full, []string{"prod\n", "n\n"}, false},
		{"work-only: exact name then yes proceeds", cleanup.WorkOnly, []string{"prod\n", "y\n"}, true},
		{"work-only: bare y at name stage refuses", cleanup.WorkOnly, []string{"y\n"}, false},
		{"web-only: single y proceeds", cleanup.WebOnly, []string{"y\n"}, true},
		{"terraform-only: single n aborts", cleanup.TerraformOnly, []string{"n\n"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testStdinReader = &lineReader{lines: tc.input}
			t.Cleanup(func() { testStdinReader = nil })

			proceed, err := confirmCleanupInteractive(context.Background(), cleanupGuardConfig(), tc.kind)
			if err != nil {
				t.Fatalf("confirmCleanupInteractive: %v", err)
			}
			if proceed != tc.proceed {
				t.Errorf("proceed = %v; want %v", proceed, tc.proceed)
			}
		})
	}
}

func resetCleanupFlags(t *testing.T) {
	t.Helper()
	savedYes, savedDryRun := cleanupYes, cleanupDryRun
	savedConfirm, savedKind := cleanupConfirmCluster, cleanupKind
	t.Cleanup(func() {
		cleanupYes, cleanupDryRun = savedYes, savedDryRun
		cleanupConfirmCluster, cleanupKind = savedConfirm, savedKind
	})
	cleanupYes, cleanupDryRun = false, false
	cleanupConfirmCluster, cleanupKind = "", string(cleanup.Full)
}

// seedCleanupWorkspace writes a loadable okdctl.yaml (cluster "prod") plus a
// sentinel file inside the okd-install workdir, then chdirs into the root.
// The sentinel lets refusal-path tests prove nothing was removed.
func seedCleanupWorkspace(t *testing.T) (sentinelPath string) {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	if err := config.NewLoader().Save(cleanupGuardConfig(), filepath.Join(root, "okdctl.yaml")); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	authDir := filepath.Join(root, workspace.WorkDirName, "cluster-config", "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinelPath = filepath.Join(authDir, "kubeconfig")
	if err := os.WriteFile(sentinelPath, []byte("credential"), 0o600); err != nil {
		t.Fatal(err)
	}
	return sentinelPath
}

func mustSurviveCleanup(t *testing.T, sentinelPath string) {
	t.Helper()
	if _, err := os.Stat(sentinelPath); err != nil {
		t.Fatalf("auth artifact was removed on a path that must not delete anything: %v", err)
	}
}

// TestRunCleanup_Wiring locks runCleanup's guard and dry-run routing: the
// dry-run branch must return before the confirm gate and any deletion, and
// the confirm gate must run before runlock/phase construction.
func TestRunCleanup_Wiring(t *testing.T) {
	t.Run("--dry-run previews and removes nothing", func(t *testing.T) {
		resetCleanupFlags(t)
		sentinel := seedCleanupWorkspace(t)
		cleanupDryRun = true
		cleanupCmd.SetContext(context.Background())

		if err := runCleanup(cleanupCmd, nil); err != nil {
			t.Fatalf("runCleanup --dry-run: %v", err)
		}
		mustSurviveCleanup(t, sentinel)
	})

	t.Run("invalid --kind is a UsageError", func(t *testing.T) {
		resetCleanupFlags(t)
		sentinel := seedCleanupWorkspace(t)
		cleanupKind = "everything"
		cleanupCmd.SetContext(context.Background())

		err := runCleanup(cleanupCmd, nil)
		if err == nil {
			t.Fatal("invalid --kind must be refused")
		}
		var usageErr *errtypes.UsageError
		if !errors.As(err, &usageErr) {
			t.Errorf("want *errtypes.UsageError (exit 64), got %T: %v", err, err)
		}
		mustSurviveCleanup(t, sentinel)
	})

	t.Run("--yes without --confirm-cluster refuses", func(t *testing.T) {
		resetCleanupFlags(t)
		sentinel := seedCleanupWorkspace(t)
		cleanupYes = true
		cleanupCmd.SetContext(context.Background())

		err := runCleanup(cleanupCmd, nil)
		if err == nil {
			t.Fatal("scripted cleanup without --confirm-cluster must be refused")
		}
		var usageErr *errtypes.UsageError
		if !errors.As(err, &usageErr) {
			t.Errorf("want *errtypes.UsageError, got %T: %v", err, err)
		}
		if !strings.Contains(err.Error(), "--confirm-cluster") {
			t.Errorf("refusal must point at --confirm-cluster: %v", err)
		}
		mustSurviveCleanup(t, sentinel)
	})

	t.Run("--yes with wrong --confirm-cluster refuses", func(t *testing.T) {
		resetCleanupFlags(t)
		sentinel := seedCleanupWorkspace(t)
		cleanupYes = true
		cleanupConfirmCluster = "staging"
		cleanupCmd.SetContext(context.Background())

		err := runCleanup(cleanupCmd, nil)
		if err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("mismatched --confirm-cluster must refuse, got: %v", err)
		}
		mustSurviveCleanup(t, sentinel)
	})

	t.Run("interactive decline cancels cleanly", func(t *testing.T) {
		resetCleanupFlags(t)
		sentinel := seedCleanupWorkspace(t)
		testStdinReader = &lineReader{lines: []string{guardTestCluster + "\n", "n\n"}}
		t.Cleanup(func() { testStdinReader = nil })
		cleanupCmd.SetContext(context.Background())

		if err := runCleanup(cleanupCmd, nil); err != nil {
			t.Fatalf("declined cleanup must exit 0, got: %v", err)
		}
		mustSurviveCleanup(t, sentinel)
	})
}
