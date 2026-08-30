package nodetypes

import "github.com/qxtaiba/okdctl/internal/config"

// VMID resolves the QEMU vmid for a role/index pair under cfg's vmid base,
// mirroring the terraform module's numbering (bootstrap=base, master=base+10+n,
// worker=base+100+n). It is the single source of this arithmetic; callers must
// stay in sync with the module or address the wrong VM.
func VMID(cfg *config.Config, role NodeRole, index int) int {
	base := cfg.Topology.VMIDBase
	if base == 0 {
		base = config.DefaultVMIDBase
	}
	switch role {
	case RoleMaster:
		return base + 10 + index
	case RoleWorker:
		return base + 100 + index
	default:
		return base
	}
}
