package nodetypes

import "github.com/qxtaiba/okdctl/internal/config"

// VMID resolves the QEMU vmid for a role/index pair under cfg's vmid base,
// mirroring the terraform module's numbering (bootstrap=base,
// masters=base+10+n, workers=base+100+n). Single owner of this arithmetic:
// power operations and status probes must agree with the module or they
// address the wrong VM.
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
