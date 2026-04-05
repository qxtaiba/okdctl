// Package install provides the install phase for OKD cluster provisioning.
package install

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/paths"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/infrastructure/proxmox"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

const (
	DefaultBootstrapTimeout    = 30 * time.Minute
	DefaultInstallTimeout      = 60 * time.Minute
	DefaultCSRApprovalInterval = 30 * time.Second
)

type Options struct {
	paths.BaseOptions
	AutoApprove         bool
	BootstrapTimeout    time.Duration
	InstallTimeout      time.Duration
	CSRApprovalInterval time.Duration
	SkipTerraform       bool
	SkipConfirmation    bool
	BootstrapIP         string
	SSHKeyPath          string
	StreamBootstrapLogs bool
}

func NewOptions(cfg *config.Config, projectRoot string) Options {
	bootstrapTimeout := DefaultBootstrapTimeout
	installTimeout := DefaultInstallTimeout

	if cfg.Deployment.BootstrapTimeout > 0 {
		bootstrapTimeout = time.Duration(cfg.Deployment.BootstrapTimeout) * time.Second
	}
	if cfg.Deployment.InstallTimeout > 0 {
		installTimeout = time.Duration(cfg.Deployment.InstallTimeout) * time.Second
	}

	sshKeyPath := ""
	if cfg.Files.SSHPublicKey != "" {
		sshKeyPath = system.ExpandPath(cfg.Files.SSHPublicKey)
		sshKeyPath = strings.TrimSuffix(sshKeyPath, ".pub")
	}

	return Options{
		BaseOptions: paths.BaseOptions{
			WorkDir:      filepath.Join(projectRoot, "okd-install"),
			ProjectRoot:  projectRoot,
			TerraformEnv: paths.GetTerraformEnv(cfg),
		},
		AutoApprove:         cfg.Deployment.AutoApprove,
		BootstrapTimeout:    bootstrapTimeout,
		InstallTimeout:      installTimeout,
		CSRApprovalInterval: DefaultCSRApprovalInterval,
		BootstrapIP:         cfg.Networking.StaticIP.Start,
		SSHKeyPath:          sshKeyPath,
		StreamBootstrapLogs: true,
	}
}


type Phase struct {
	paths.BasePhase
}

func New(exec *executor.Executor, logger utils.Logger, version string) *Phase {
	return &Phase{
		BasePhase: paths.NewBasePhase(exec, logger, version),
	}
}

func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts Options) error {
	orchestrator := distribution.NewOrchestrator(
		p.newDeployInfraStep(cfg, opts),
		p.newWaitBootstrapStep(cfg, opts),
		p.newStartWorkersStep(cfg, opts),
		p.newSetupKubeconfigStep(opts),
		p.newValidateAccessStep(opts),
		p.newMonitorInstallStep(cfg, opts),
		p.newSetupAccessStep(opts),
	)
	orchestrator.SetLogger(p.Log)

	if err := orchestrator.Run(ctx); err != nil {
		return err
	}

	return nil
}

func (p *Phase) DeployInfrastructure(ctx context.Context, cfg *config.Config, opts Options) error {
	terraformDir := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", opts.TerraformEnv)
	tfvarsFile := filepath.Join(terraformDir, "terraform.tfvars")

	if opts.Debug {
		p.Log.Debug(fmt.Sprintf("terraform: directory %s", terraformDir))
		p.Log.Debug(fmt.Sprintf("terraform: tfvars file %s", tfvarsFile))
	}

	if !system.DirExists(terraformDir) {
		return fmt.Errorf("terraform environment directory not found: %s", terraformDir)
	}

	if !system.FileExists(tfvarsFile) {
		return fmt.Errorf("terraform.tfvars not found: %s - run setup first", tfvarsFile)
	}

	prov := proxmox.New(
		proxmox.WithProjectRoot(opts.ProjectRoot),
		proxmox.WithLogger(p.Log),
		proxmox.WithEnv(p.Exec.Env),
	)
	if err := prov.Connect(ctx, cfg); err != nil {
		return utils.WrapError("failed to connect to Proxmox", err)
	}
	defer func() { _ = prov.Disconnect(ctx) }()

	provOpts := proxmox.ProvisionOptions{
		AutoApprove:  opts.AutoApprove,
		ProjectRoot:  opts.ProjectRoot,
		TerraformEnv: opts.TerraformEnv,
	}

	_, err := prov.Provision(ctx, cfg, provOpts)
	if err != nil {
		return err
	}

	return nil
}

func (p *Phase) SetupKubeconfig(clusterDir string) error {
	kubeconfigPath := filepath.Join(clusterDir, "auth", "kubeconfig")
	if !system.FileExists(kubeconfigPath) {
		return fmt.Errorf("kubeconfig not found at %s", kubeconfigPath)
	}
	p.Exec.Env = append(p.Exec.Env, "KUBECONFIG="+kubeconfigPath)
	p.Log.Info(fmt.Sprintf("kubeconfig: configured KUBECONFIG=%s for phase executor", kubeconfigPath))
	return nil
}

