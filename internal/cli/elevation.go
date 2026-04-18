package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
)

// rootRequiredCmds lists subcommands that perform privileged operations
// (writing to /etc, /usr/local/bin, /var/www/html, managing systemd units,
// configuring firewalls). When invoked without euid=0, the CLI re-execs
// itself under sudo before cobra's RunE fires so the body runs single-UID.
var rootRequiredCmds = map[string]bool{
	"deploy":         true,
	"destroy":        true,
	"cleanup":        true,
	"update-ingress": true,
}

// ensureRoot is wired into the root cobra command's PersistentPreRunE. It
// is a no-op for unprivileged commands (wizard, doctor, --help, --version)
// and for invocations that already have euid=0. For root-required commands
// invoked as a non-root user, it re-execs the same binary under sudo with
// the same args and environment. syscall.Exec replaces the current process,
// so a successful re-exec never returns. The euid=0 check prevents re-exec
// loops.
func ensureRoot(cmd *cobra.Command) error {
	if !rootRequiredCmds[cmd.Name()] {
		return nil
	}
	if os.Geteuid() == 0 {
		return nil
	}
	sudoPath, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("%s requires root and sudo is not installed; run as root", cmd.Name())
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve own binary: %w", err)
	}
	args := append([]string{"sudo", "--", self}, os.Args[1:]...)
	return syscall.Exec(sudoPath, args, os.Environ())
}
