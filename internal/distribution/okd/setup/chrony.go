package setup

import (
	"encoding/base64"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// chronyMachineConfigRoles are the MachineConfig pools chrony must target —
// the MCO applies pool-scoped configs only to matching nodes, so shipping
// only "master" would leave workers on their unmanaged default chrony.conf.
var chronyMachineConfigRoles = []string{string(nodetypes.RoleMaster), string(nodetypes.RoleWorker)}

// generateChronyManifests writes a chrony MachineConfig for each node pool,
// pointed at cfg.Networking.NTPServer (defaulting to the bastion's ignition
// server IP when unset).
func (p *Phase) generateChronyManifests(cfg *config.Config, clusterDir string) error {
	server := cfg.Networking.NTPServer
	if server == "" {
		server = cfg.HTTPServer.IgnitionServerIP
	}

	chronyConf, err := templates.RenderChronyConf(templates.ChronyConfData{Server: server})
	if err != nil {
		return fmt.Errorf("render chrony.conf: %w", err)
	}
	source := "data:text/plain;charset=utf-8;base64," + base64.StdEncoding.EncodeToString([]byte(chronyConf))

	openshiftDir := filepath.Join(clusterDir, openshiftSubdir)
	if err := system.EnsureDir(openshiftDir); err != nil {
		return fmt.Errorf("ensure openshift manifests directory: %w", err)
	}

	for _, role := range chronyMachineConfigRoles {
		name := fmt.Sprintf("99-%s-chrony-configuration", role)
		manifest, err := templates.RenderChronyMachineConfig(templates.ChronyMachineConfigData{
			Role:   role,
			Name:   name,
			Source: source,
		})
		if err != nil {
			return fmt.Errorf("render chrony machineconfig: %w", err)
		}
		path := filepath.Join(openshiftDir, name+".yaml")
		if err := system.AtomicWriteString(path, manifest, 0o644); err != nil {
			return fmt.Errorf("write chrony machineconfig: %w", err)
		}
	}

	p.Log.Info("chrony: machineconfigs generated", "server", server)
	return nil
}
