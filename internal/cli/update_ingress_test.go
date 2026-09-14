package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

func TestBuildConvertConfirm_YesTrue(t *testing.T) {
	if !buildConvertConfirm(context.Background(), true)(nil) {
		t.Error("yes=true must confirm without consulting stdin")
	}
}

func TestBuildConvertConfirm_YesFalse(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"y answer", "y\n", true},
		{"n answer", "n\n", false},
		{"EOF", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old := testStdinReader
			testStdinReader = strings.NewReader(tc.input)
			t.Cleanup(func() { testStdinReader = old })

			fn := buildConvertConfirm(context.Background(), false)
			if got := fn([]string{"default"}); got != tc.want {
				t.Errorf("input %q: want %v, got %v", tc.input, tc.want, got)
			}
		})
	}
}

func resetUpdateIngressFlags(t *testing.T) {
	t.Helper()
	savedYes, savedKeep, savedDryRun := updateIngressYes, updateIngressKeepHAProxy, updateIngressDryRun
	savedConfirm := updateIngressConfirmCluster
	t.Cleanup(func() {
		updateIngressYes, updateIngressKeepHAProxy, updateIngressDryRun = savedYes, savedKeep, savedDryRun
		updateIngressConfirmCluster = savedConfirm
	})
	updateIngressYes, updateIngressKeepHAProxy, updateIngressDryRun = false, false, false
	updateIngressConfirmCluster = ""
}

func seedUpdateIngressWorkspace(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	if err := config.NewLoader().Save(guardConfig(), filepath.Join(root, "okdctl.yaml")); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

// TestRunUpdateIngress_ConfirmBoxPrintsEvenOnYesGateRefusal pins that the
// confirm box reaches stderr ahead of confirmClusterMatches, so a --yes run
// refused there for a wrong --confirm-cluster still shows the box.
func TestRunUpdateIngress_ConfirmBoxPrintsEvenOnYesGateRefusal(t *testing.T) {
	resetUpdateIngressFlags(t)
	seedUpdateIngressWorkspace(t)
	updateIngressYes = true
	updateIngressConfirmCluster = "staging"

	var stderr bytes.Buffer
	updateIngressCmd.SetContext(context.Background())
	updateIngressCmd.SetErr(&stderr)
	t.Cleanup(func() { updateIngressCmd.SetErr(nil) })

	err := runUpdateIngress(updateIngressCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched --confirm-cluster must refuse, got: %v", err)
	}

	out := stderr.String()
	if !strings.Contains(out, "confirm ingress update") {
		t.Errorf("confirm box must print even on a --yes run refused at the cluster-match gate:\n%s", out)
	}
}

// TestRunUpdateIngress_DryRunPreviewsWithoutConfirmation pins that --dry-run
// prints the boxed preview to stdout and returns before any confirmation gate.
func TestRunUpdateIngress_DryRunPreviewsWithoutConfirmation(t *testing.T) {
	resetUpdateIngressFlags(t)
	seedUpdateIngressWorkspace(t)
	updateIngressDryRun = true
	updateIngressCmd.SetContext(context.Background())
	var stdout bytes.Buffer
	updateIngressCmd.SetOut(&stdout)
	t.Cleanup(func() { updateIngressCmd.SetOut(nil) })

	if err := runUpdateIngress(updateIngressCmd, nil); err != nil {
		t.Fatalf("update-ingress --dry-run: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{"would", "IngressControllers", "--dry-run"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run preview missing %q:\n%s", want, out)
		}
	}
}
