package nodetypes

import "github.com/qxtaiba/okdctl/internal/config"

// ProxmoxNode resolves Terraform's index-based placement and default-node padding.
func ProxmoxNode(cfg *config.Config, role NodeRole, index int) string {
	px := cfg.Provider.Proxmox
	if px == nil {
		return ""
	}
	var selected []string
	switch role {
	case RoleMaster:
		selected = px.ControlPlaneNodes
	case RoleWorker:
		selected = px.WorkerNodes
	}
	if index >= 0 && index < len(selected) && selected[index] != "" {
		return selected[index]
	}
	return px.Node
}
