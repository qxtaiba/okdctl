package setup

import (
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// fstrimMachineConfigRoles are the MachineConfig pools the fstrim workaround
// must target — the MCO applies pool-scoped configs only to matching nodes,
// so shipping only "master" would leave workers on FCOS's broken stock
// fstrim.timer (coreos/fedora-coreos-tracker#468).
var fstrimMachineConfigRoles = []string{string(nodetypes.RoleMaster), string(nodetypes.RoleWorker)}

// generateFstrimManifests writes an fstrim MachineConfig for each node pool.
// FCOS ships no /etc/fstab, so the stock fstrim.timer's `fstrim --fstab`
// unit fails and guest-side TRIM never runs; every disk in the Proxmox
// module is provisioned with discard=on, so this silently defeats thin-pool
// reclaim. The MachineConfig masks the stock timer and ships an okdctl-owned
// replacement that trims explicit FCOS mountpoints.
func (p *Phase) generateFstrimManifests(clusterDir string) error {
	openshiftDir := filepath.Join(clusterDir, openshiftSubdir)
	if err := system.EnsureDir(openshiftDir); err != nil {
		return fmt.Errorf("ensure openshift manifests directory: %w", err)
	}

	for _, role := range fstrimMachineConfigRoles {
		name := fmt.Sprintf("99-%s-fstrim-configuration", role)
		manifest, err := templates.RenderFstrimMachineConfig(templates.FstrimMachineConfigData{
			Role: role,
			Name: name,
		})
		if err != nil {
			return fmt.Errorf("render fstrim machineconfig: %w", err)
		}
		path := filepath.Join(openshiftDir, name+".yaml")
		if err := system.AtomicWriteString(path, manifest, 0o644); err != nil {
			return fmt.Errorf("write fstrim machineconfig: %w", err)
		}
	}

	p.Log.Info("fstrim: machineconfigs generated")
	return nil
}
