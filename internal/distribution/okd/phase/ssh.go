package phase

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// ProxmoxBareHost strips any port suffix or URL scheme from the host so the
// result can be passed directly to ssh. Proxmox hosts in config may appear as
// "host:8006" or "https://host".
func ProxmoxBareHost(host string) string {
	if idx := strings.Index(host, "://"); idx != -1 {
		host = host[idx+3:]
	}
	if strings.Contains(host, ":") {
		if h, _, err := net.SplitHostPort(host); err == nil {
			return h
		}
	}
	return host
}

// SSHRun runs a single command on root@host over SSH, reusing the shared
// flag set (-o StrictHostKeyChecking=accept-new -o BatchMode=yes).
// Non-zero exit codes do not produce an error; only transport failures do.
func SSHRun(ctx context.Context, exec *executor.Executor, host, cmd string) (*executor.Result, error) {
	result, err := exec.Run(ctx, "ssh",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
		"root@"+host,
		cmd,
	)
	if err != nil {
		return result, fmt.Errorf("ssh %s: %w", host, err)
	}
	return result, nil
}

// SSHRunArgv passes each argv element to ssh as a separate non-option
// argument. ssh(1) joins the trailing args with spaces and sends one
// command string to the remote login shell — argv mode does NOT bypass
// the shell. Callers MUST validate every atom for shell metacharacters
// before calling; pveshRun is the canonical example.
func SSHRunArgv(ctx context.Context, exec *executor.Executor, host string, argv ...string) (*executor.Result, error) {
	args := make([]string, 0, 4+len(argv))
	args = append(args, "-o", "StrictHostKeyChecking=accept-new", "-o", "BatchMode=yes", "root@"+host)
	args = append(args, argv...)
	result, err := exec.Run(ctx, "ssh", args...)
	if err != nil {
		return result, fmt.Errorf("ssh %s: %w", host, err)
	}
	return result, nil
}
