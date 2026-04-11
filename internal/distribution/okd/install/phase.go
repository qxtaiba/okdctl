// Package install provides the install phase for OKD cluster provisioning.
package install

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
	"github.com/qxtaiba/okdctl/internal/utils/system"
)

const (
	DefaultBootstrapTimeout    = 30 * time.Minute
	DefaultInstallTimeout      = 60 * time.Minute
	DefaultCSRApprovalInterval = 30 * time.Second
)

type Options struct {
	phase.BaseOptions
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
		BaseOptions: phase.BaseOptions{
			WorkDir:      filepath.Join(projectRoot, "okd-install"),
			ProjectRoot:  projectRoot,
			TerraformEnv: phase.GetTerraformEnv(cfg),
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
	phase.BasePhase
}

func New(exec *executor.Executor, logger *slog.Logger, version string) *Phase {
	return &Phase{
		BasePhase: phase.NewBasePhase(exec, logger, version),
	}
}

func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts *Options) error {
	orchestrator := distribution.NewOrchestrator(distribution.BuildSteps(p.installSteps(cfg, opts))...)
	orchestrator.SetLogger(p.Log)

	if err := orchestrator.Run(ctx); err != nil {
		return err
	}

	return nil
}

func (p *Phase) DeployInfrastructure(ctx context.Context, cfg *config.Config, opts *Options) error {
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
		return fmt.Errorf("failed to connect to Proxmox: %w", err)
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

// SetupKubeconfig appends KUBECONFIG=<path> to Exec.Env so subprocesses
// launched via p.Exec.Run inherit it. K8sClient reads os.Environ at
// construction and will NOT see this — callers constructing a K8sClient
// after this runs must pass cluster.WithKubeconfig explicitly.
func (p *Phase) SetupKubeconfig(clusterDir string) error {
	kubeconfigPath := filepath.Join(clusterDir, "auth", "kubeconfig")
	if !system.FileExists(kubeconfigPath) {
		return fmt.Errorf("kubeconfig not found at %s", kubeconfigPath)
	}
	p.Exec.Env = append(p.Exec.Env, "KUBECONFIG="+kubeconfigPath)
	p.Log.Info(fmt.Sprintf("kubeconfig: configured KUBECONFIG=%s for phase executor", kubeconfigPath))
	return nil
}
