package cli

import (
	"errors"
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

// annotationValueTrue is the canonical truthy value for cobra annotations
// (e.g. requiresRoot). Cobra annotations are map[string]string, so callers
// must compare against a string; this constant is the single source of truth.
const annotationValueTrue = "true"

// lookPath is the exec.LookPath indirection used by ensureRoot.
// Tests replace it with a stub to avoid real PATH lookups.
var lookPath = exec.LookPath

type elevAction int

const (
	elevAllow   elevAction = iota // already root and command requires it, or no root needed
	elevReject                    // euid=0 on a command that must not run as root
	elevElevate                   // must re-exec under sudo
)

// requiresRoot returns true if cmd carries the requiresRoot annotation or
// any ancestor is in rootRequiredCmds. --dry-run escapes the gate so
// `okdctl destroy --dry-run` prints the preview without a sudo prompt.
func requiresRoot(cmd *cobra.Command) bool {
	if dry, err := cmd.Flags().GetBool(flagDryRun); err == nil && dry {
		return false
	}
	if cmd.Annotations[annotationKeyRequiresRoot] == annotationValueTrue {
		return true
	}
	for c := cmd; c != nil; c = c.Parent() {
		if slices.Contains(rootRequiredCmds, c.Name()) {
			return true
		}
	}
	return false
}

// elevationDecision returns the action ensureRoot should take for the given
// command and effective UID.
func elevationDecision(cmd *cobra.Command, euid int) elevAction {
	needsRoot := requiresRoot(cmd)
	if euid == 0 {
		if needsRoot {
			return elevAllow
		}
		return elevReject
	}
	if !needsRoot {
		return elevAllow
	}
	return elevElevate
}

// ensureRoot is wired into the root cobra command's PersistentPreRunE.
// Policy:
//
//	euid=0 ∧  requiresRoot → allow (re-exec'd process running the privileged body)
//	euid=0 ∧ !requiresRoot → reject (e.g. `sudo okdctl status`)
//	euid≠0 ∧  requiresRoot → re-exec under sudo
//	euid≠0 ∧ !requiresRoot → allow
func ensureRoot(cmd *cobra.Command) error {
	switch elevationDecision(cmd, os.Geteuid()) {
	case elevAllow:
		return nil
	case elevReject:
		return &errtypes.AuthError{
			Msg: "do not run as root/sudo; this tool escalates internally",
			Err: os.ErrPermission,
		}
	}
	sudoPath, err := lookPath("sudo")
	if err != nil {
		return &errtypes.AuthError{
			Msg: fmt.Sprintf("%s requires root and sudo is not installed; run as root", cmd.Name()),
			Err: errors.Join(err, errtypes.ErrSudoMissing),
		}
	}
	self, err := os.Executable()
	if err != nil {
		return &errtypes.ConfigError{Msg: "resolve own binary", Err: err}
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
