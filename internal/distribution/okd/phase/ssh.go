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
