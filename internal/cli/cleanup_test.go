package cli

import (
	"context"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
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
