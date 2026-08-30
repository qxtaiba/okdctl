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

			proceed, err := confirmCleanupInteractive(context.Background(), guardConfig(), tc.kind)
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

// seedCleanupWorkspace's sentinel file proves refusal paths delete nothing.
func seedCleanupWorkspace(t *testing.T) (sentinelPath string) {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	if err := config.NewLoader().Save(guardConfig(), filepath.Join(root, "okdctl.yaml")); err != nil {
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
