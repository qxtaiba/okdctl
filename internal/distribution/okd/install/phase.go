// Package install provides the install phase for OKD cluster provisioning.
package install

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
	"github.com/qxtaiba/okdctl/internal/system"
)

// Default timeouts and intervals for the install phase. Overridable via
// Deployment.BootstrapTimeout / Deployment.InstallTimeout in the Config.
const (
	DefaultBootstrapTimeout    = 30 * time.Minute
	DefaultInstallTimeout      = 60 * time.Minute
	DefaultCSRApprovalInterval = 30 * time.Second
)

// Options configures an install run: timeouts, CSR approval cadence, and
// the bootstrap IP/SSH details used to stream bootstrap logs.
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

// NewOptions builds install Options from cfg, applying deployment-level
// timeout overrides and resolving the SSH key path for bootstrap log
// streaming.
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

// Phase drives the install flow: openshift-install wrapper, bootstrap
// monitor, and cluster-up poll.
type Phase struct {
	phase.BasePhase
	// startMonitorCmd, when non-nil, replaces the default subprocess
	// start-and-wait used by MonitorInstallation. Tests inject a pure-Go
	// implementation to avoid spawning real processes.
	startMonitorCmd func(ctx context.Context, clusterDir string) (<-chan error, func(), error)
}

// New constructs an install Phase with the given version tag and options.
func New(version string, opts ...phase.BasePhaseOption) *Phase {
	bp := phase.NewBasePhase(version, opts...)
	bp.Log = bp.Log.With("phase", "install")
	return &Phase{BasePhase: bp}
}

// Execute runs the install phase step sequence and returns each step's
// result. A non-nil error means orchestration stopped early.
func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts *Options) ([]distribution.StepResult, error) {
	orchestrator := distribution.NewOrchestrator(distribution.BuildSteps(p.installSteps(cfg, opts))...)
	orchestrator.SetLogger(p.Log)
	orchestrator.SetMetricsRecorder(p.Recorder)

	if err := orchestrator.Run(ctx); err != nil {
		return orchestrator.Results(), err
	}

	return orchestrator.Results(), nil
}

// DeployInfrastructure applies the generated Terraform plan against Proxmox
// to provision the bootstrap and node VMs.
func (p *Phase) DeployInfrastructure(ctx context.Context, cfg *config.Config, opts *Options) error {
	terraformDir := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", opts.TerraformEnv)
	tfvarsFile := filepath.Join(terraformDir, "terraform.tfvars")

	if opts.Debug {
		p.Log.Debug("terraform: directory", "path", terraformDir)
		p.Log.Debug("terraform: tfvars file", "path", tfvarsFile)
	}

	if !system.DirExists(terraformDir) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("terraform environment directory not found: %s", terraformDir)}
	}

	if !system.FileExists(tfvarsFile) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("terraform.tfvars not found: %s - run setup first", tfvarsFile)}
	}

	prov := proxmox.New(
		proxmox.WithProjectRoot(opts.ProjectRoot),
		proxmox.WithLogger(p.Log),
		proxmox.WithEnv(p.Exec.Env),
		proxmox.WithProgressReporter(p.Reporter),
		proxmox.WithSSHExec(p.Exec),
	)
	defer prov.ZeroizeEnv()
	if err := prov.Connect(ctx, cfg); err != nil {
		return &errtypes.NetworkError{Msg: "failed to connect to Proxmox", Err: err}
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
func (p *Phase) SetupKubeconfig(ctx context.Context, clusterDir string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("setup kubeconfig: %w", err)
	}
	kubeconfigPath := filepath.Join(clusterDir, "auth", "kubeconfig")
	if !system.FileExists(kubeconfigPath) {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("kubeconfig not found at %s", kubeconfigPath)}
	}
	p.Exec.Env = append(p.Exec.Env, "KUBECONFIG="+kubeconfigPath)
	p.Log.Info("kubeconfig: configured for phase executor", "path", kubeconfigPath)
	return nil
}
