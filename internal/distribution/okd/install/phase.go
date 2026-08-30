// Package install drives the install phase: terraform-up, bootstrap monitor,
// CSR approval, and cluster-operator settle.
package install

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// Default timeouts and intervals for the install phase, overridable via
// Deployment.BootstrapTimeout/InstallTimeout.
const (
	DefaultBootstrapTimeout    = 30 * time.Minute
	DefaultInstallTimeout      = 60 * time.Minute
	DefaultCSRApprovalInterval = 30 * time.Second
)

// Options configures an install run: timeouts and CSR approval cadence.
type Options struct {
	phase.BaseOptions
	AutoApprove         bool
	BootstrapTimeout    time.Duration
	InstallTimeout      time.Duration
	CSRApprovalInterval time.Duration
	SkipTerraform       bool
}

// NewOptions builds install Options from cfg, applying deployment-level timeout overrides.
func NewOptions(cfg *config.Config, projectRoot string) Options {
	bootstrapTimeout := DefaultBootstrapTimeout
	installTimeout := DefaultInstallTimeout

	if cfg.Deployment.BootstrapTimeout > 0 {
		bootstrapTimeout = time.Duration(cfg.Deployment.BootstrapTimeout) * time.Second
	}
	if cfg.Deployment.InstallTimeout > 0 {
		installTimeout = time.Duration(cfg.Deployment.InstallTimeout) * time.Second
	}

	return Options{
		BaseOptions:         phase.NewBaseOptions(cfg, projectRoot),
		AutoApprove:         cfg.Deployment.AutoApprove,
		BootstrapTimeout:    bootstrapTimeout,
		InstallTimeout:      installTimeout,
		CSRApprovalInterval: DefaultCSRApprovalInterval,
	}
}

// Phase drives the install flow: openshift-install wrapper, bootstrap monitor, and cluster-up poll.
type Phase struct {
	phase.BasePhase
	// startMonitorCmd, when non-nil, replaces the default subprocess
	// start-and-wait; tests inject a pure-Go stub.
	startMonitorCmd func(ctx context.Context, clusterDir string) (<-chan error, func(), error)
}

// New constructs an install Phase with the given options.
func New(opts ...phase.BasePhaseOption) *Phase {
	bp := phase.NewBasePhase(opts...)
	bp.Log = bp.Log.With("phase", "install")
	return &Phase{BasePhase: bp}
}

// Execute runs the install phase steps. cfg must be the same cfg passed to
// NewOptions — opts is derived from it and the two aren't re-validated here.
func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts *Options) ([]distribution.StepResult, error) {
	orchestrator := distribution.NewOrchestrator(distribution.BuildSteps(p.installSteps(cfg, opts))...)
	orchestrator.SetLogger(p.Log)
	orchestrator.SetMetricsRecorder(p.Recorder)

	err := orchestrator.Run(ctx)
	return orchestrator.Results(), err
}

// StepDefs returns the ordered step definitions for cfg/opts without running
// them, used by Provisioner.DeploySteps for the deploy --dry-run listing.
func (p *Phase) StepDefs(cfg *config.Config, opts *Options) []distribution.StepDef {
	return p.installSteps(cfg, opts)
}

// DeployInfrastructure applies the generated Terraform plan against Proxmox to
// provision the bootstrap and node VMs.
func (p *Phase) DeployInfrastructure(ctx context.Context, cfg *config.Config, opts *Options) error {
	terraformDir := workspace.TerraformEnvDir(opts.ProjectRoot, opts.TerraformEnv)
	tfvarsFile := filepath.Join(terraformDir, "terraform.tfvars")

	p.Log.Debug("terraform: directory", "path", terraformDir)
	p.Log.Debug("terraform: tfvars file", "path", tfvarsFile)

	if !system.DirExists(terraformDir) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("terraform environment directory not found: %s", terraformDir)}
	}

	if !system.FileExists(tfvarsFile) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("terraform.tfvars not found: %s - run setup first", tfvarsFile)}
	}

	prov := proxmox.New(
		proxmox.WithProjectRoot(opts.ProjectRoot),
		proxmox.WithLogger(p.Log),
		proxmox.WithEnv(p.Exec.SnapshotEnv()),
		proxmox.WithProgressReporter(p.Reporter),
		proxmox.WithSSHExec(p.Exec),
	)
	defer prov.ZeroizeEnv()
	if err := prov.Connect(ctx, cfg); err != nil {
		return &errtypes.NetworkError{Msg: "connect to Proxmox", Err: err}
	}
	defer func() { _ = prov.Disconnect(ctx) }()

	provOpts := proxmox.ProvisionOptions{
		AutoApprove:  opts.AutoApprove,
		ProjectRoot:  opts.ProjectRoot,
		TerraformEnv: opts.TerraformEnv,
	}

	return prov.Provision(ctx, cfg, provOpts)
}

// SetupKubeconfig appends KUBECONFIG=<path> to the phase executor env so
// p.Exec.Run subprocesses inherit it. A cluster.Client not built via
// cluster.WithExecutor(p.Exec) reads os.Environ only at construction and
// won't see this — pass cluster.WithKubeconfig explicitly.
func (p *Phase) SetupKubeconfig(ctx context.Context, clusterDir string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("setup kubeconfig: %w", err)
	}
	kubeconfigPath := workspace.KubeconfigPath(clusterDir)
	if !system.FileExists(kubeconfigPath) {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("kubeconfig not found at %s", kubeconfigPath)}
	}
	p.Exec.AppendEnv("KUBECONFIG=" + kubeconfigPath)
	p.Log.Info("kubeconfig: configured for phase executor", "path", kubeconfigPath)
	return nil
}
