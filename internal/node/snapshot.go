package node

import (
	"context"
	"fmt"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox/hostssh"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// SnapshotCreateOptions tunes a node snapshot create.
type SnapshotCreateOptions struct {
	Name         string
	Description  string
	DrainTimeout string
	SkipDrain    bool
}

// defaultSnapshotName generates a letter-led, pve-configid-valid name for a
// caller that did not supply one.
func defaultSnapshotName() string {
	return "okdctl-" + time.Now().UTC().Format("20060102-150405")
}

func (r *Runner) snapshotTimeout() time.Duration {
	if r.SnapshotTaskTimeout > 0 {
		return r.SnapshotTaskTimeout
	}
	return DefaultSnapshotTaskTimeout
}

// CreateSnapshot snapshots target's disks via pvesh, cordoning and draining
// first when the node is Ready and the caller has not asked to skip it — a
// NotReady node is snapshotted directly, since cordoning teaches nothing new
// and a drain would only spin with nowhere to reschedule its pods. Every
// snapshot is disk-only: qemu-guest-agent is disabled fleet-wide (or its
// state could not be probed), so the operator is warned it is
// crash-consistent rather than equivalent to a clean shutdown — unconditionally,
// including under dry-run. Returns the name actually used: opts.Name, or an
// auto-generated timestamped name when opts.Name is empty.
func (r *Runner) CreateSnapshot(ctx context.Context, target string, opts SnapshotCreateOptions) (_ string, err error) {
	if r.Proxmox == nil {
		return "", &errtypes.ClusterError{Msg: "snapshot needs Proxmox SSH access, but no Proxmox host is configured"}
	}
	vmid, role, ready, verr := r.resolveVMID(ctx, target)
	if verr != nil {
		return "", verr
	}

	agentEnabled, probeErr := r.Snapshot.VMAgentEnabled(ctx, r.Proxmox, vmid)
	if probeErr != nil {
		r.Log.Warn("node: could not probe qemu-guest-agent state; assuming disabled", "node", target, "err", probeErr)
	}
	if !agentEnabled {
		r.Log.Warn("node: snapshot is crash-consistent only (qemu-guest-agent disabled or unavailable), not equivalent to a clean shutdown", "node", target)
	}

	name := opts.Name
	if name == "" {
		name = defaultSnapshotName()
	}

	if r.DryRun {
		r.Log.Info("node: dry-run — would snapshot vm", "node", target, "vmid", vmid, "name", name)
		return name, nil
	}

	if ready && !opts.SkipDrain {
		if derr := r.cordonAndDrain(ctx, OpSnapshot, target, opts.DrainTimeout, role == nodetypes.RoleMaster, Step("")); derr != nil {
			return "", derr
		}
		// Uncordon runs whether the snapshot below succeeds or fails, so a
		// snapshot failure never leaves the node needlessly unschedulable on
		// top of the original problem. If the snapshot itself succeeded and
		// this uncordon then fails, that failure becomes the op's result —
		// the caller must know the node is unexpectedly still cordoned. Only
		// a clean uncordon clears the OpSnapshot marker cordonAndDrain wrote:
		// a failure leaves it in place (matching remove/resize precedent) as
		// an operator-visible "left cordoned" trail, correctly attributed to
		// snapshot rather than misread as a stuck remove.
		defer func() {
			if uerr := r.Cluster.Uncordon(ctx, target); uerr != nil {
				if err == nil {
					err = uerr
				} else {
					r.Log.Warn("node: uncordon after snapshot failed", "node", target, "err", uerr)
				}
			}
			if err == nil {
				if cerr := clearOpMarker(r.marker()); cerr != nil {
					r.Log.Warn("node: op marker cleanup failed", "err", cerr)
				}
			}
		}()
	}

	stop := r.startProgress(fmt.Sprintf("snapshotting %s", target))
	cerr := r.Snapshot.CreateSnapshot(ctx, r.Proxmox, vmid, name, opts.Description, r.snapshotTimeout())
	stop()
	if cerr != nil {
		return "", &errtypes.ClusterError{Msg: fmt.Sprintf("create snapshot %s for %s", name, target), Err: cerr}
	}

	r.Log.Info("node: snapshot created", "node", target, "name", name)
	return name, nil
}

// ListSnapshots returns target's Proxmox snapshots. Read-only; runs the same
// under dry-run and the real path.
func (r *Runner) ListSnapshots(ctx context.Context, target string) ([]hostssh.SnapshotInfo, error) {
	if r.Proxmox == nil {
		return nil, &errtypes.ClusterError{Msg: "snapshot list needs Proxmox SSH access, but no Proxmox host is configured"}
	}
	vmid, _, _, err := r.resolveVMID(ctx, target)
	if err != nil {
		return nil, err
	}
	snapshots, err := r.Snapshot.ListSnapshots(ctx, r.Proxmox, vmid)
	if err != nil {
		return nil, &errtypes.ClusterError{Msg: fmt.Sprintf("list snapshots for %s", target), Err: err}
	}
	return snapshots, nil
}

// RollbackSnapshot restores target's disks to snapname; pvesh passes -start 1,
// so the VM is powered back on regardless of its prior power state. A master
// rollback is quorum-sensitive — a crash-consistent snapshot can leave etcd's
// Raft term or rook-ceph's OSD state stale relative to peers that kept
// running — so the op refuses to even start against an already-unhealthy
// quorum (pre-gate, real runs only: a dry-run must never block on a gate whose
// purpose is to wait) and re-verifies health before returning the node to
// service (post-gate). Any failure from cordonAndDrain onward leaves the node
// cordoned; the returned error names the failing stage, and only a clean
// post-gate reaches the final (best-effort) uncordon, which is also what
// clears the OpSnapshot marker cordonAndDrain wrote.
func (r *Runner) RollbackSnapshot(ctx context.Context, target, snapname string) error {
	if r.Proxmox == nil {
		return &errtypes.ClusterError{Msg: "snapshot rollback needs Proxmox SSH access, but no Proxmox host is configured"}
	}
	vmid, role, _, err := r.resolveVMID(ctx, target)
	if err != nil {
		return err
	}
	isMaster := role == nodetypes.RoleMaster

	if isMaster && !r.DryRun {
		if err := r.waitEtcdHealthy(ctx, "pre-rollback-"+target); err != nil {
			return err
		}
		if err := r.waitCephHealthy(ctx, "pre-rollback-"+target); err != nil {
			return err
		}
	}

	if r.DryRun {
		r.Log.Info("node: dry-run — would roll back vm to snapshot (auto-starts the vm)", "node", target, "vmid", vmid, "name", snapname)
		return nil
	}

	if err := r.cordonAndDrain(ctx, OpSnapshot, target, "", isMaster, Step("")); err != nil {
		return err
	}

	stop := r.startProgress(fmt.Sprintf("rolling back %s to snapshot %s", target, snapname))
	rerr := r.Snapshot.RollbackSnapshot(ctx, r.Proxmox, vmid, snapname, r.snapshotTimeout())
	stop()
	if rerr != nil {
		return &errtypes.ClusterError{
			Msg: fmt.Sprintf("roll back %s to snapshot %s (node left cordoned; re-run after resolving the cause)", target, snapname),
			Err: rerr,
		}
	}

	if err := r.waitNodeReady(ctx, target); err != nil {
		return err
	}

	if isMaster {
		if err := r.waitEtcdHealthy(ctx, "post-rollback-"+target); err != nil {
			return err
		}
		if err := r.waitCephHealthy(ctx, "post-rollback-"+target); err != nil {
			return err
		}
	}

	if err := r.Cluster.Uncordon(ctx, target); err != nil {
		r.Log.Warn("node: uncordon after rollback failed", "node", target, "err", err)
	}

	if err := clearOpMarker(r.marker()); err != nil {
		r.Log.Warn("node: op marker cleanup failed", "err", err)
	}

	r.Log.Info("node: rolled back", "node", target, "name", snapname)
	return nil
}

// DeleteSnapshot removes snapname from target. It does not touch VM power
// state or cordon status — deleting a snapshot artifact has no effect on the
// running guest.
func (r *Runner) DeleteSnapshot(ctx context.Context, target, snapname string) error {
	if r.Proxmox == nil {
		return &errtypes.ClusterError{Msg: "snapshot delete needs Proxmox SSH access, but no Proxmox host is configured"}
	}
	vmid, _, _, err := r.resolveVMID(ctx, target)
	if err != nil {
		return err
	}

	if r.DryRun {
		r.Log.Info("node: dry-run — would delete snapshot", "node", target, "vmid", vmid, "name", snapname)
		return nil
	}

	stop := r.startProgress(fmt.Sprintf("deleting snapshot %s for %s", snapname, target))
	derr := r.Snapshot.DeleteSnapshot(ctx, r.Proxmox, vmid, snapname, r.snapshotTimeout())
	stop()
	if derr != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("delete snapshot %s for %s", snapname, target), Err: derr}
	}

	r.Log.Info("node: snapshot deleted", "node", target, "name", snapname)
	return nil
}
