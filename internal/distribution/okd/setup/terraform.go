package setup

import (
	"context"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/provision"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// GenerateTerraformVars renders terraform.tfvars into the environment
// directory derived from cfg.
func (p *Phase) GenerateTerraformVars(ctx context.Context, cfg *config.Config, opts *Options) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	envDir := workspace.TerraformEnvDir(opts.ProjectRoot, cfg.TerraformEnvName())
	// A stale sentinel would silently re-skip bootstrap; only deploy removes it
	// here — node-lifecycle re-renders must preserve it.
	_ = system.SafeRemove(filepath.Join(envDir, workspace.BootstrapStateSentinelFile))
	return provision.WriteTerraformVars(cfg, envDir)
}
