package setup

import (
	"context"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/provision"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// GenerateTerraformVars renders terraform.tfvars for the Proxmox provider
// into the environment directory derived from cfg.
func (p *Phase) GenerateTerraformVars(ctx context.Context, cfg *config.Config, opts *Options) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	envDir := workspace.TerraformEnvDir(opts.ProjectRoot, cfg.TerraformEnvName())
	// A stale postinstall sentinel would override the regenerated
	// bootstrap_enabled=true and silently skip the bootstrap VM. Deploy is the
	// only caller that should resurrect bootstrap, so the removal lives here,
	// not in WriteTerraformVars — node-lifecycle re-renders must preserve it.
	_ = system.SafeRemove(filepath.Join(envDir, workspace.BootstrapStateSentinelFile))
	return provision.WriteTerraformVars(cfg, envDir)
}
