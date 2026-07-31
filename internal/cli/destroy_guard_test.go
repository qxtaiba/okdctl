package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/testutil"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// resetDestroyFlags zeroes the destroy command's package-level flag variables
// and restores the caller-time values on cleanup, so tests can exercise the
// real validateDestroyFlagCombos/buildDestroyOptions/confirmDestroyInteractive
// (not mirrors of them) without leaking state across tests.
func resetDestroyFlags(t *testing.T) {
	t.Helper()
	savedYes, savedKeepISOs, savedDryRun := destroyYes, destroyKeepISOs, destroyDryRun
	savedConfirm, savedOnly := destroyConfirmCluster, destroyOnly
	savedSkipTF, savedSkipCleanup, savedSkipFW := destroySkipTerraform, destroySkipCleanup, destroySkipFirewall
	savedTargets := destroyTargets
	t.Cleanup(func() {
		destroyYes, destroyKeepISOs, destroyDryRun = savedYes, savedKeepISOs, savedDryRun
		destroyConfirmCluster, destroyOnly = savedConfirm, savedOnly
		destroySkipTerraform, destroySkipCleanup, destroySkipFirewall = savedSkipTF, savedSkipCleanup, savedSkipFW
		destroyTargets = savedTargets
	})
	destroyYes, destroyKeepISOs, destroyDryRun = false, false, false
	destroyConfirmCluster, destroyOnly = "", ""
	destroySkipTerraform, destroySkipCleanup, destroySkipFirewall = false, false, false
	destroyTargets = nil
}

const (
	guardTestCluster = "prod"
	guardTestTarget  = "module.okd_cluster.proxmox_virtual_environment_vm.worker[1]"
)

func destroyGuardConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = guardTestCluster
	return cfg
}

func TestValidateDestroyFlagCombos_TargetedRequiresConfirmCluster(t *testing.T) {
	resetDestroyFlags(t)
	cfg := destroyGuardConfig()
	destroyTargets = []string{guardTestTarget}

	err := validateDestroyFlagCombos(cfg)
	if err == nil {
		t.Fatal("targeted destroy without --confirm-cluster must be refused")
	}
	var usageErr *errtypes.UsageError
	if !errors.As(err, &usageErr) {
		t.Errorf("want *errtypes.UsageError (exit 64), got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), `"prod"`) {
		t.Errorf("refusal should name the cluster the operator must confirm: %v", err)
	}

	destroyConfirmCluster = guardTestCluster
	if err := validateDestroyFlagCombos(cfg); err != nil {
		t.Errorf("targeted destroy with --confirm-cluster must pass: %v", err)
	}
}

func TestValidateDestroyFlagCombos_DryRunRejectsSkipFlags(t *testing.T) {
	cases := []struct {
		name  string
		set   func()
		wants []string
	}{
		{"skip-terraform", func() { destroySkipTerraform = true }, []string{"--skip-terraform"}},
		{"skip-cleanup", func() { destroySkipCleanup = true }, []string{"--skip-cleanup"}},
		{"skip-firewall", func() { destroySkipFirewall = true }, []string{"--skip-firewall"}},
		{"all three", func() {
			destroySkipTerraform, destroySkipCleanup, destroySkipFirewall = true, true, true
		}, []string{"--skip-terraform", "--skip-cleanup", "--skip-firewall"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetDestroyFlags(t)
			destroyDryRun = true
			tc.set()

			err := validateDestroyFlagCombos(destroyGuardConfig())
			if err == nil {
				t.Fatal("--dry-run with skip flags must be refused")
			}
			var usageErr *errtypes.UsageError
			if !errors.As(err, &usageErr) {
				t.Errorf("want *errtypes.UsageError, got %T: %v", err, err)
			}
			for _, want := range tc.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal must name %s: %v", want, err)
				}
			}
		})
	}
}

func TestValidateDestroyFlagCombos_DryRunAloneAndPlainDestroyPass(t *testing.T) {
	resetDestroyFlags(t)
	cfg := destroyGuardConfig()

	if err := validateDestroyFlagCombos(cfg); err != nil {
		t.Errorf("unscoped destroy with no flags must pass: %v", err)
	}
	destroyDryRun = true
	if err := validateDestroyFlagCombos(cfg); err != nil {
		t.Errorf("--dry-run alone must pass: %v", err)
	}
}

// TestBuildDestroyOptions_ScopedForcesBastionTeardownOff locks the scoped-
// destroy invariant on the real buildDestroyOptions: with --target set, host
// cleanup, firewall teardown, and ISO removal are forced off regardless of the
// operator's flags, so a partial destroy never tears down the bastion services
// a still-running control plane depends on.
func TestBuildDestroyOptions_ScopedForcesBastionTeardownOff(t *testing.T) {
	resetDestroyFlags(t)
	cfg := destroyGuardConfig()
	destroyTargets = []string{guardTestTarget}

	opts := buildDestroyOptions(cfg, t.TempDir())
	if !opts.SkipCleanup || !opts.SkipFirewall || !opts.KeepISOs {
		t.Errorf("scoped destroy must force cleanup/firewall/iso off: SkipCleanup=%v SkipFirewall=%v KeepISOs=%v",
			opts.SkipCleanup, opts.SkipFirewall, opts.KeepISOs)
	}
	if len(opts.TerraformTargets) != 1 || opts.TerraformTargets[0] != guardTestTarget {
		t.Errorf("targets must pass through to the destroy phase: %v", opts.TerraformTargets)
	}
	if !opts.AutoApprove {
		t.Error("CLI-confirmed destroy must auto-approve terraform (confirmation already happened)")
	}
}

func TestBuildDestroyOptions_UnscopedPassesFlagsThrough(t *testing.T) {
	resetDestroyFlags(t)
	cfg := destroyGuardConfig()
	destroySkipFirewall = true
	destroyKeepISOs = true

	opts := buildDestroyOptions(cfg, t.TempDir())
	if opts.SkipCleanup {
		t.Error("unscoped destroy must not force SkipCleanup")
	}
	if !opts.SkipFirewall || !opts.KeepISOs {
		t.Errorf("operator flags must pass through: SkipFirewall=%v KeepISOs=%v", opts.SkipFirewall, opts.KeepISOs)
	}
	if len(opts.TerraformTargets) != 0 {
		t.Errorf("unscoped destroy must carry no targets: %v", opts.TerraformTargets)
	}
}

// lineReader delivers exactly one line per Read call, mirroring a TTY in
// canonical mode. promptForLine wraps the shared reader in a fresh
// bufio.Reader on every prompt, so a plain strings.Reader carrying both
// answers would have its second line swallowed by the first prompt's
// buffered lookahead — exactly what happens to pasted-ahead input on a real
// terminal, where the gate then fails closed (declines).
type lineReader struct{ lines []string }

func (r *lineReader) Read(p []byte) (int, error) {
	if len(r.lines) == 0 {
		return 0, io.EOF
	}
	line := r.lines[0]
	r.lines = r.lines[1:]
	return copy(p, line), nil
}

// TestConfirmDestroyInteractive drives the real two-stage interactive gate:
// an unscoped destroy demands the exact cluster name and then a y/N; a typo,
// a bare "y" at the name stage, or a decline all abort. A scoped destroy
// (--target already passed --confirm-cluster) skips the typed-name stage.
func TestConfirmDestroyInteractive(t *testing.T) {
	cases := []struct {
		name    string
		scoped  bool
		input   []string
		proceed bool
	}{
		{"wrong cluster name refuses", false, []string{"prod-oops\n"}, false},
		{"bare y at name stage refuses", false, []string{"y\n"}, false},
		{"exact name then yes proceeds", false, []string{"prod\n", "y\n"}, true},
		{"exact name then no aborts", false, []string{"prod\n", "n\n"}, false},
		{"exact name then default-empty aborts", false, []string{"prod\n", "\n"}, false},
		{"case-mismatched name refuses", false, []string{"PROD\n", "y\n"}, false},
		{"scoped skips name stage", true, []string{"y\n"}, true},
		{"scoped still requires the y", true, []string{"n\n"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetDestroyFlags(t)
			if tc.scoped {
				destroyTargets = []string{guardTestTarget}
			}
			testStdinReader = &lineReader{lines: tc.input}
			t.Cleanup(func() { testStdinReader = nil })

			proceed, err := confirmDestroyInteractive(context.Background(), destroyGuardConfig())
			if err != nil {
				t.Fatalf("confirmDestroyInteractive: %v", err)
			}
			if proceed != tc.proceed {
				t.Errorf("proceed = %v; want %v", proceed, tc.proceed)
			}
		})
	}
}

// seedDestroyWorkspace writes a loadable okdctl.yaml (cluster "prod") into a
// temp project root, chdirs into it, and plants a fake terraform on PATH
// that records a marker file if it is ever executed — the refusal paths
// under test must never reach terraform.
func seedDestroyWorkspace(t *testing.T) (markerPath string) {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	cfg := destroyGuardConfig()
	if err := config.NewLoader().Save(cfg, filepath.Join(root, "okdctl.yaml")); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	markerPath = filepath.Join(t.TempDir(), "terraform-ran")
	testutil.InstallFakeBin(t, "terraform", "#!/bin/sh\ntouch "+markerPath+"\nexit 0\n")
	return markerPath
}

func mustNotRunTerraform(t *testing.T, markerPath string) {
	t.Helper()
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatal("terraform was executed on a path that must refuse before any infra action")
	}
}

// TestRunDestroy_DryRunPreviewsWithoutConfirmation drives the real dry-run
// path end to end against a fake terraform: no prompt, no confirm flags, and
// the preview runs init plus a -destroy plan.
func TestRunDestroy_DryRunPreviewsWithoutConfirmation(t *testing.T) {
	resetDestroyFlags(t)
	root := t.TempDir()
	t.Chdir(root)
	if err := config.NewLoader().Save(destroyGuardConfig(), filepath.Join(root, "okdctl.yaml")); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	tfDir := filepath.Join(root, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(tfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	argvLog := filepath.Join(t.TempDir(), "tf-argv.log")
	t.Setenv("TF_ARGV_LOG", argvLog)
	testutil.InstallFakeBin(t, "terraform",
		"#!/bin/sh\n[ -n \"$TF_ARGV_LOG\" ] && echo \"$@\" >> \"$TF_ARGV_LOG\"\nexit 0\n")
	destroyDryRun = true
	destroyCmd.SetContext(context.Background())

	if err := runDestroy(destroyCmd, nil); err != nil {
		t.Fatalf("destroy --dry-run: %v", err)
	}
	argv, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatalf("fake terraform never ran: %v", err)
	}
	if !strings.Contains(string(argv), "init") {
		t.Errorf("dry-run must run terraform init, got argv:\n%s", argv)
	}
	if !strings.Contains(string(argv), "-destroy") {
		t.Errorf("dry-run must run a -destroy plan, got argv:\n%s", argv)
	}
}

// TestRunDestroy_ConfirmGateWiring locks the confirm gate INTO runDestroy:
// the guards are unit-tested elsewhere, but deleting the
// confirmClusterMatches call from runDestroy would previously have passed
// the entire suite.
func TestRunDestroy_ConfirmGateWiring(t *testing.T) {
	t.Run("--yes without --confirm-cluster refuses", func(t *testing.T) {
		resetDestroyFlags(t)
		marker := seedDestroyWorkspace(t)
		destroyYes = true
		destroyCmd.SetContext(context.Background())

		err := runDestroy(destroyCmd, nil)
		if err == nil {
			t.Fatal("scripted destroy without --confirm-cluster must be refused")
		}
		var cfgErr *errtypes.ConfigError
		if !errors.As(err, &cfgErr) {
			t.Errorf("want *errtypes.ConfigError, got %T: %v", err, err)
		}
		if !strings.Contains(err.Error(), "--confirm-cluster") {
			t.Errorf("refusal must point at --confirm-cluster: %v", err)
		}
		mustNotRunTerraform(t, marker)
	})

	t.Run("--yes with wrong --confirm-cluster refuses", func(t *testing.T) {
		resetDestroyFlags(t)
		marker := seedDestroyWorkspace(t)
		destroyYes = true
		destroyConfirmCluster = "staging"
		destroyCmd.SetContext(context.Background())

		err := runDestroy(destroyCmd, nil)
		if err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("mismatched --confirm-cluster must refuse, got: %v", err)
		}
		mustNotRunTerraform(t, marker)
	})

	t.Run("interactive decline at y/N cancels cleanly", func(t *testing.T) {
		resetDestroyFlags(t)
		marker := seedDestroyWorkspace(t)
		testStdinReader = &lineReader{lines: []string{guardTestCluster + "\n", "n\n"}}
		t.Cleanup(func() { testStdinReader = nil })
		destroyCmd.SetContext(context.Background())

		if err := runDestroy(destroyCmd, nil); err != nil {
			t.Fatalf("declined destroy must exit 0, got: %v", err)
		}
		mustNotRunTerraform(t, marker)
	})

	t.Run("interactive wrong cluster name cancels cleanly", func(t *testing.T) {
		resetDestroyFlags(t)
		marker := seedDestroyWorkspace(t)
		testStdinReader = &lineReader{lines: []string{"prod-oops\n"}}
		t.Cleanup(func() { testStdinReader = nil })
		destroyCmd.SetContext(context.Background())

		if err := runDestroy(destroyCmd, nil); err != nil {
			t.Fatalf("typo at the name gate must cancel, not destroy: %v", err)
		}
		mustNotRunTerraform(t, marker)
	})
}

// captureStderrLog redirects the tui stderr logger into a buffer via the
// same ConfigureLoggers seam resetLoggingState uses, so logutil.Warn/Info
// output can be asserted without swapping the global facade handler.
func captureStderrLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	if err := tui.ConfigureLoggers("info", "text", io.Discard, &buf, false); err != nil {
		t.Fatalf("capture loggers: %v", err)
	}
	t.Cleanup(func() {
		if err := tui.ConfigureLoggers("info", "text", os.Stdout, os.Stderr, false); err != nil {
			t.Errorf("restore loggers: %v", err)
		}
	})
	return &buf
}

// TestRunDestroy_PreambleSurfacesInFlightNodeOp pins that an in-flight
// node-op marker is warned about BEFORE the confirmation gate (the operator
// declines at the typed-name stage and the warning has already fired), and
// that the warning names the op and its target. The same call sits ahead of
// confirmClusterMatches, so a --yes run gets the identical warning in its
// log.
func TestRunDestroy_PreambleSurfacesInFlightNodeOp(t *testing.T) {
	resetDestroyFlags(t)
	marker := seedDestroyWorkspace(t)
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	writeNodeOpMarker(t, root, guardTestCluster, "add", "worker9", "wait-join")

	buf := captureStderrLog(t)

	testStdinReader = &lineReader{lines: []string{"prod-oops\n"}}
	t.Cleanup(func() { testStdinReader = nil })
	destroyCmd.SetContext(context.Background())

	if err := runDestroy(destroyCmd, nil); err != nil {
		t.Fatalf("declined destroy must exit 0, got: %v", err)
	}
	mustNotRunTerraform(t, marker)

	out := buf.String()
	for _, want := range []string{"node op is in flight", "op=add", "target=worker9"} {
		if !strings.Contains(out, want) {
			t.Errorf("destroy preamble log missing %q:\n%s", want, out)
		}
	}
}

// TestAnnounceInFlightNodeOp_PlanPreamble covers the shared preview helper
// okdctl plan wires in: a marker for this cluster warns with op/target; a
// foreign-cluster marker stays silent.
func TestAnnounceInFlightNodeOp_PlanPreamble(t *testing.T) {
	cases := []struct {
		name          string
		markerCluster string
		wantWarn      bool
	}{
		{"same cluster warns", guardTestCluster, true},
		{"different cluster stays silent", "someone-else", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeNodeOpMarker(t, root, tc.markerCluster, "remove", "worker2", "drain")

			buf := captureStderrLog(t)

			announceInFlightNodeOp(root, destroyGuardConfig())

			got := strings.Contains(buf.String(), "node op is in flight")
			if got != tc.wantWarn {
				t.Errorf("warned = %v, want %v; log:\n%s", got, tc.wantWarn, buf.String())
			}
		})
	}
}
