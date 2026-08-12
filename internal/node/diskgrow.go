package node

import (
	"context"
	"fmt"
	"strings"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// growOSDiskScript runs in-guest (chroot /host) after the hypervisor-level
// disk grow: rescan the SCSI device so the kernel sees the new size, grow
// partition 4 into it, grow the xfs filesystem live. Device-path
// assumptions (scsi0 → /dev/sda, root on partition 4) are invariants of
// this module's images — see the design spec. growpart exits 1 for
// NOCHANGE (already grown — the resume/idempotency case), which the ||
// guard converts to success; any other non-zero rc aborts via set -e.
const growOSDiskScript = `set -e
echo 1 > /sys/class/block/sda/device/rescan
growpart /dev/sda 4 || [ $? -eq 1 ]
xfs_growfs /var`

// debugRunner is the slice of cluster.Client the grower drives; an
// interface so tests record the argv without a live cluster.
type debugRunner interface {
	Run(ctx context.Context, args ...string) (*executor.Result, error)
}

// DebugNodeGrower is the production diskGrower: it grows the OS filesystem
// via a privileged `oc debug node/<n>` pod chrooted into the host. Output
// is captured by the executor, never streamed — safe under the TUI's
// AltScreen.
type DebugNodeGrower struct {
	Runner debugRunner
}

// GrowOSDisk runs the in-guest grow script on node via oc debug.
func (g *DebugNodeGrower) GrowOSDisk(ctx context.Context, node string) error {
	result, err := g.Runner.Run(ctx, "debug", "node/"+node, "-q", "--",
		"chroot", "/host", "sh", "-c", growOSDiskScript)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		detail := strings.TrimSpace(result.Stderr)
		if detail == "" {
			detail = strings.TrimSpace(result.Stdout)
		}
		return fmt.Errorf("in-guest disk grow on %s exited %d: %s", node, result.ExitCode, detail)
	}
	return nil
}
