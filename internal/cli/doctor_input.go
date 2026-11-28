package cli

import (
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/model"
	"github.com/qxtaiba/okdctl/internal/proxmox"
)

// preflightInput derives doctor's footprint through the node model; the VM
// tag comes only from a written cluster record, since preview identities are
// not the cluster's, and the ISO size stays 0 because the install builds the
// agent ISO after doctor runs.
func preflightInput(cfg *config.Config, configPath string) (proxmox.PreflightInput, error) {
	infraID, recorded, err := recordedInfraID(configPath, cfg.Cluster.Name)
	if err != nil {
		return proxmox.PreflightInput{}, err
	}
	inv, err := model.Build(cfg, infraID)
	if err != nil {
		return proxmox.PreflightInput{}, &errtypes.ConfigError{Msg: "derive node inventory: " + err.Error(), Err: err}
	}
	var tag string
	if recorded {
		tag = infraID
	}
	return proxmox.PreflightInputFor(&inv, &cfg.Provider.Proxmox, tag, 0), nil
}
