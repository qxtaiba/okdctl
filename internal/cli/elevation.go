package cli

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
)

// rootRequiredCmds lists subcommand names that perform privileged operations
// (writing to /etc, /usr/local/bin, /var/www/html, managing systemd units,
// configuring firewalls). When invoked without euid=0, the CLI re-execs
// itself under sudo before cobra's RunE fires so the body runs single-UID.
//
// Matching walks the cobra parent chain, so a future nested layout like
// `okdctl cluster deploy` still triggers the gate as long as `deploy`
// stays in this slice.
var rootRequiredCmds = []string{"deploy", "destroy", "cleanup", "update-ingress"}

// requiresRoot returns true if cmd carries the requiresRoot annotation or
// any ancestor is in rootRequiredCmds. --dry-run escapes the gate so
// `okdctl destroy --dry-run` prints the preview without a sudo prompt.
func requiresRoot(cmd *cobra.Command) bool {
	if dry, err := cmd.Flags().GetBool("dry-run"); err == nil && dry {
		return false
	}
	if cmd.Annotations["requiresRoot"] == "true" {
		return true
	}
	for c := cmd; c != nil; c = c.Parent() {
		if slices.Contains(rootRequiredCmds, c.Name()) {
			return true
		}
	}
	return false
}

// ensureRoot is wired into the root cobra command's PersistentPreRunE. It
// is a no-op for unprivileged commands (wizard, doctor, --help, --version)
// and for invocations that already have euid=0. For root-required commands
// invoked as a non-root user, it re-execs the same binary under sudo with
// the same args and environment. syscall.Exec replaces the current process,
// so a successful re-exec never returns. The euid=0 check prevents re-exec
// loops.
func ensureRoot(cmd *cobra.Command) error {
	if !requiresRoot(cmd) {
		return nil
	}
	if os.Geteuid() == 0 {
		return nil
	}
	sudoPath, err := exec.LookPath("sudo")
	if err != nil {
		return &errtypes.AuthError{
			Msg: fmt.Sprintf("%s requires root and sudo is not installed; run as root", cmd.Name()),
			Err: err,
		}
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve own binary: %w", err)
	}
	args := append([]string{"sudo", "--", self}, os.Args[1:]...)
	// args are forwarded to sudo as an argv slice (no shell interpolation),
	// and the `--` separator pins the binary. cobra validated the args
	// before this PreRunE runs; callers cannot inject flags into sudo itself.
	//
	// Filter the environment to the same allowlist used by Executor
	// subprocesses so unrelated tokens (AWS, GCP, shell plumbing) do not
	// reach the privileged re-exec'd process.
	return syscall.Exec(sudoPath, args, executor.FilterParentEnv(executor.DefaultEnvAllowlist)) //nolint:gosec // argv slice, no shell
}
