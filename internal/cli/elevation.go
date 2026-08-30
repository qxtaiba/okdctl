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

// rootRequiredCmds lists subcommands needing sudo re-exec; matching walks the
// parent chain so nested layouts still trigger the gate.
var rootRequiredCmds = []string{cmdNameDeploy, cmdNameDestroy, cmdNameCleanup, "update-ingress"}

const annotationValueTrue = "true"

// lookPath is exec.LookPath, indirected so tests can stub it.
var lookPath = exec.LookPath

type elevAction int

const (
	elevAllow   elevAction = iota // already root and command requires it, or no root needed
	elevReject                    // euid=0 on a command that must not run as root
	elevElevate                   // must re-exec under sudo
)

// requiresRoot reports whether cmd or any ancestor requires root; --dry-run
// always escapes the gate.
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

// ensureRoot backs PersistentPreRunE's elevation gate: euid=0 allows only if
// requiresRoot, euid≠0 re-execs via sudo if requiresRoot else allows.
// OKDCTL_WIZARD_DEMO=1 skips the re-exec for the demo; privileged steps
// still require root, so the knob can't silently degrade a real deploy.
func ensureRoot(cmd *cobra.Command) error {
	if os.Getenv(wizardDemoEnv) != "" {
		return nil
	}
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
	// argv-only exec (no shell); env filtered to Executor's allowlist so extra
	// secrets/tokens don't reach the re-exec'd process.
	return syscall.Exec(sudoPath, args, executor.FilterParentEnv(executor.DefaultEnvAllowlist)) //nolint:gosec // argv slice, no shell
}
