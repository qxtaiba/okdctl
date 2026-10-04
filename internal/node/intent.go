package node

import (
	"fmt"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// OpIntent records the approved mutation parameters independently of its checkpoint.
type OpIntent struct {
	Scope             string `json:"scope"`
	MemoryMB          int    `json:"memory_mb,omitempty"`
	CPU               int    `json:"cpu,omitempty"`
	OSDiskGB          int    `json:"os_disk_gb,omitempty"`
	RequestedMemoryMB int    `json:"requested_memory_mb,omitempty"`
	RequestedCPU      int    `json:"requested_cpu,omitempty"`
	RequestedOSDiskGB int    `json:"requested_os_disk_gb,omitempty"`
	DiskOnly          bool   `json:"disk_only,omitempty"`
	SkipDrain         bool   `json:"skip_drain,omitempty"`
	ForceStorage      bool   `json:"force_storage,omitempty"`
	DrainTimeout      string `json:"drain_timeout,omitempty"`
	AddCount          int    `json:"add_count,omitempty"`
	StartWorkerCount  int    `json:"start_worker_count,omitempty"`
}

func (r *Runner) resizeIntent(scope ResizeScope, role nodetypes.NodeRole, opts ResizeOptions) *OpIntent {
	sizing := r.Cfg.Topology.Workers
	if role == nodetypes.RoleMaster {
		sizing = r.Cfg.Topology.ControlPlane
	}
	intent := &OpIntent{
		Scope: fmt.Sprintf("%s/%s", scope.Role, scope.Node), MemoryMB: opts.MemoryMB, CPU: opts.CPU, OSDiskGB: opts.OSDiskGB,
		RequestedMemoryMB: opts.MemoryMB, RequestedCPU: opts.CPU, RequestedOSDiskGB: opts.OSDiskGB, DiskOnly: opts.OSDiskGB > 0 && opts.MemoryMB <= 0 && opts.CPU <= 0, SkipDrain: opts.SkipDrain,
	}
	if intent.MemoryMB <= 0 {
		intent.MemoryMB = sizing.MemoryMB
	}
	if intent.CPU <= 0 {
		intent.CPU = sizing.CPU
	}
	if intent.OSDiskGB <= 0 {
		intent.OSDiskGB = sizing.DiskGB
	}
	return intent
}

func removeIntent(target string, opts RemoveOptions) *OpIntent {
	timeout := opts.DrainTimeout
	if timeout == "" {
		timeout = defaultDrainTimeout
	}
	if duration, err := time.ParseDuration(timeout); err == nil {
		timeout = duration.String()
	}
	return &OpIntent{Scope: target, SkipDrain: opts.SkipDrain, ForceStorage: opts.ForceStorage, DrainTimeout: timeout}
}

func (r *Runner) addIntentStart(count int) (int, error) {
	start := r.Cfg.Topology.Workers.Count
	if r.DryRun && !r.ResumePreview {
		return start, nil
	}
	saved, err := ReadOpMarker(r.workDir, r.Cfg.Cluster.Name)
	if err != nil {
		return 0, err
	}
	if saved == nil || saved.Op != OpAdd || saved.Intent == nil || saved.Intent.AddCount != count {
		return start, nil
	}
	old := saved.Intent.StartWorkerCount
	if start != old && start != old+count {
		return 0, &errtypes.ConfigError{Msg: "worker count changed since interrupted add"}
	}
	return old, nil
}

func (r *Runner) addIntent(start, count int) *OpIntent {
	sizing := r.Cfg.Topology.Workers
	return &OpIntent{Scope: "workers", StartWorkerCount: start, AddCount: count, MemoryMB: sizing.MemoryMB, CPU: sizing.CPU, OSDiskGB: sizing.DiskGB}
}
