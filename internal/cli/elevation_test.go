package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/platform"
)

func newCmd(name string) *cobra.Command {
	child := &cobra.Command{Use: name, RunE: func(_ *cobra.Command, _ []string) error { return nil }}
	root := &cobra.Command{Use: "okdctl"}
	root.AddCommand(child)
	return child
}

func newDryRunCmd(name string) *cobra.Command {
	child := newCmd(name)
	child.Flags().Bool("dry-run", false, "")
	_ = child.Flags().Set("dry-run", "true")
	return child
}

func TestElevationDecision(t *testing.T) {
	deployCmd := newCmd("deploy")
	wizardCmd := newCmd("wizard")
	dryDeployCmd := newDryRunCmd("deploy")

	cases := []struct {
		name string
		cmd  *cobra.Command
		euid int
		want elevAction
	}{
		{"root+requiresRoot", deployCmd, 0, elevAllow},
		{"root+noRoot", wizardCmd, 0, elevReject},
		{"nonroot+requiresRoot", deployCmd, 1000, elevElevate},
		{"nonroot+noRoot", wizardCmd, 1000, elevAllow},
		{"dryRun+requiresRoot+nonroot", dryDeployCmd, 1000, elevAllow},
		// dry-run flips requiresRoot false; euid=0 ∧ !requiresRoot is always reject.
		{"dryRun+requiresRoot+root", dryDeployCmd, 0, elevReject},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := elevationDecision(tc.cmd, tc.euid)
			if got != tc.want {
				t.Fatalf("elevationDecision(%s, euid=%d) = %v, want %v", tc.cmd.Name(), tc.euid, got, tc.want)
			}
		})
	}
}

func TestRequiresRoot(t *testing.T) {
	destroyDry := newDryRunCmd("destroy")

	destroyFalseDry := newCmd("destroy")
	destroyFalseDry.Flags().Bool("dry-run", false, "")

	deployNoFlag := newCmd("deploy")
	statusCmd := newCmd("status")

	cases := []struct {
		name string
		cmd  *cobra.Command
		want bool
	}{
		{"destroy dry-run=true escapes gate", destroyDry, false},
		{"destroy dry-run=false stays in gate", destroyFalseDry, true},
		{"deploy without dry-run flag triggers gate", deployNoFlag, true},
		{"status not in rootRequiredCmds", statusCmd, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := requiresRoot(tc.cmd)
			if got != tc.want {
				t.Fatalf("requiresRoot(%s) = %v, want %v", tc.cmd.Name(), got, tc.want)
			}
		})
	}
}

func TestEnsureRoot_SudoNotFound(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("only meaningful when test process is non-root")
	}
	stubHostSupported(t, nil)
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })
	lookPath = func(_ string) (string, error) { return "", exec.ErrNotFound }

	cmd := newCmd("deploy")
	err := ensureRoot(cmd)
	var authErr *errtypes.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("want AuthError, got %T: %v", err, err)
	}
	if !errors.Is(err, errtypes.ErrSudoMissing) {
		t.Fatalf("want errtypes.ErrSudoMissing in chain; exitCodeFor would return 5 instead of 71")
	}
}

func stubHostSupported(t *testing.T, err error) {
	t.Helper()
	orig := hostSupportedFn
	hostSupportedFn = func() error { return err }
	t.Cleanup(func() { hostSupportedFn = orig })
}

func unsupportedHostErr() error {
	return fmt.Errorf("%w %q: okdctl deploys from rhel-family bastions only", platform.ErrUnsupportedOS, "ubuntu")
}

func TestEnsureRoot_DeployRefusesUnsupportedHostBeforeSudo(t *testing.T) {
	t.Setenv(wizardDemoEnv, "")
	stubHostSupported(t, unsupportedHostErr())
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })
	lookPath = func(_ string) (string, error) {
		t.Error("sudo lookup reached on an unsupported host")
		return "", exec.ErrNotFound
	}

	for _, cmd := range []*cobra.Command{newCmd("deploy"), newDryRunCmd("deploy")} {
		err := ensureRoot(cmd)
		var cfgErr *errtypes.ConfigError
		if !errors.As(err, &cfgErr) || !errors.Is(err, platform.ErrUnsupportedOS) {
			t.Fatalf("want ConfigError wrapping ErrUnsupportedOS, got %T: %v", err, err)
		}
		if !strings.Contains(err.Error(), `"ubuntu"`) || !strings.Contains(err.Error(), "rhel-family bastions only") {
			t.Errorf("refusal must name the host and the supported family: %v", err)
		}
	}
}

func TestEnsureRoot_TeardownIsNotHostGated(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("only meaningful when test process is non-root")
	}
	t.Setenv(wizardDemoEnv, "")
	stubHostSupported(t, unsupportedHostErr())
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })
	lookPath = func(_ string) (string, error) { return "", exec.ErrNotFound }

	err := ensureRoot(newCmd("destroy"))
	if errors.Is(err, platform.ErrUnsupportedOS) {
		t.Fatalf("destroy must stay usable from any host, got: %v", err)
	}
	if !errors.Is(err, errtypes.ErrSudoMissing) {
		t.Fatalf("want the sudo stage to be reached, got: %v", err)
	}
}

func TestEnsureRoot_RootRejectedForWizard(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("only meaningful when test process is root")
	}
	cmd := newCmd("wizard")
	err := ensureRoot(cmd)
	var authErr *errtypes.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("want AuthError, got %T: %v", err, err)
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("want os.ErrPermission in chain, got %v", err)
	}
}
