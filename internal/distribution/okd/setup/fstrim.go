package setup

import (
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// fstrimMachineConfigRoles must include both pools; the MCO applies configs
// only to matching nodes (coreos/fedora-coreos-tracker#468).
var fstrimMachineConfigRoles = []string{string(nodetypes.RoleMaster), string(nodetypes.RoleWorker)}

// generateFstrimManifests replaces FCOS's broken stock fstrim.timer (no
// /etc/fstab) so thin-pool discard reclaim runs.
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
