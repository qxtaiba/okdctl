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
	// Acknowledge overrides a stranded marker from any in-flight op; snapshot
	// is non-resumable so nothing is exempt (see refuseForeignMarker).
	Acknowledge bool
}

// SnapshotRollbackOptions tunes a node snapshot rollback.
type SnapshotRollbackOptions struct {
	// Acknowledge overrides a stranded marker from any in-flight op; see
	// SnapshotCreateOptions.Acknowledge.
	Acknowledge bool
}

// defaultSnapshotName generates a letter-led, pve-configid-valid name for
// callers that didn't supply one.
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
// first unless the node is NotReady or the caller skips it. It returns the name
// used (opts.Name, or an auto-generated one) and always warns that the snapshot
// is crash-consistent only, since qemu-guest-agent is disabled fleet-wide.
func (r *Runner) CreateSnapshot(ctx context.Context, target string, opts SnapshotCreateOptions) (_ string, err error) {
	if r.Proxmox == nil {
		return "", &errtypes.ClusterError{Msg: "snapshot needs Proxmox SSH access, but no Proxmox host is configured"}
	}
	if !r.DryRun {
		if err := r.refuseForeignMarker(opts.Acknowledge); err != nil {
			return "", err
		}
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
		// Uncordon always runs; a post-snapshot-success uncordon failure
		// becomes the result, and only a clean uncordon clears the OpSnapshot
		// marker — a failure leaves an operator-visible cordoned trail
		// attributed to snapshot, not a stuck remove.
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

	r.Log.Info("node: creating snapshot", "node", target, "name", name)
	stop := r.startProgress(fmt.Sprintf("snapshotting %s", target))
	cerr := r.Snapshot.CreateSnapshot(ctx, r.Proxmox, vmid, name, opts.Description, r.snapshotTimeout())
	stop()
	if cerr != nil {
		return "", &errtypes.ClusterError{Msg: fmt.Sprintf("create snapshot %s for %s", name, target), Err: cerr}
	}

	r.Log.Info("node: snapshot created", "node", target, "name", name)
	return name, nil
}

// ListSnapshots returns target's Proxmox snapshots; read-only, so it runs the
// same under dry-run and the real path.
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

// RollbackSnapshot restores target's disks to snapname and powers the VM back
// on regardless of its prior power state. For masters it gates on
// etcd/rook-ceph health before and after, since a crash-consistent snapshot can
// leave their state stale relative to peers; any failure after cordonAndDrain
// leaves the node cordoned and is returned as the result.
func (r *Runner) RollbackSnapshot(ctx context.Context, target, snapname string, opts SnapshotRollbackOptions) error {
	if r.Proxmox == nil {
		return &errtypes.ClusterError{Msg: "snapshot rollback needs Proxmox SSH access, but no Proxmox host is configured"}
	}
	if !r.DryRun {
		if err := r.refuseForeignMarker(opts.Acknowledge); err != nil {
			return err
		}
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

	r.Log.Info("node: rolling back to snapshot", "node", target, "name", snapname)

	if err := r.cordonAndDrain(ctx, OpSnapshot, target, "", isMaster, Step("")); err != nil {
		return err
	}

	stop := r.startProgress(fmt.Sprintf("rolling back %s to snapshot %s", target, snapname))
	rerr := r.Snapshot.RollbackSnapshot(ctx, r.Proxmox, vmid, snapname, r.snapshotTimeout())
	stop()
	if rerr != nil {
		return &errtypes.ClusterError{
			Msg: fmt.Sprintf("roll back %s to snapshot %s (node left cordoned; re-run with --acknowledge-interrupted-op after resolving the cause)", target, snapname),
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

	if uerr := r.Cluster.Uncordon(ctx, target); uerr != nil {
		return &errtypes.ClusterError{
			Msg: fmt.Sprintf("uncordon %s after rollback to snapshot %s succeeded (node left cordoned; uncordon manually with 'oc adm uncordon %s')", target, snapname, target),
			Err: uerr,
		}
	}

	if err := clearOpMarker(r.marker()); err != nil {
		r.Log.Warn("node: op marker cleanup failed", "err", err)
	}

	r.Log.Info("node: rolled back", "node", target, "name", snapname)
	return nil
}

// DeleteSnapshot removes snapname from target; it does not touch VM power state or cordon status.
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
