package deployment

import (
	"context"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/install"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okd-proxmox-cli/internal/logging"
)

// Step IDs for deployment phases.
const (
	StepIDPrepare   distribution.StepID = "prepare"
	StepIDInstall   distribution.StepID = "install"
	StepIDConfigure distribution.StepID = "configure"
)

// Executor orchestrates deployment using the Orchestrator.
// It wraps the OKD provisioner and provides step-based execution.
type Executor struct {
	cfg             *config.Config
	provisioner     *okd.Provisioner
	projectRoot     string
	logger          logging.Logger
	configureResult *postinstall.Result // Captured from configure step
}

// ExecutorOption configures an Executor.
type ExecutorOption func(*Executor)

// WithLogger sets a custom logger for the executor.
func WithLogger(l logging.Logger) ExecutorOption {
	return func(e *Executor) {
		if l != nil {
			e.logger = l
		}
	}
}

// NewExecutor creates a new deployment executor.
func NewExecutor(
	cfg *config.Config,
	provisioner *okd.Provisioner,
	projectRoot string,
	opts ...ExecutorOption,
) *Executor {
	e := &Executor{
		cfg:         cfg,
		provisioner: provisioner,
		projectRoot: projectRoot,
		logger:      logging.NoopLogger(),
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// Execute runs all deployment phases using the Orchestrator.
// Returns the deployment result containing IPs and other info gathered during configuration.
func (e *Executor) Execute(ctx context.Context) (*Result, error) {
	steps := e.buildSteps()
	orchestrator := distribution.NewOrchestrator(steps...)
	orchestrator.SetLogger(e.logger)

	if err := orchestrator.Run(ctx); err != nil {
		return nil, err
	}

	// Build result from captured configure result
	var result *Result
	if e.configureResult != nil {
		result = &Result{
			RouterLBIP:           e.configureResult.RouterLBIP,
			GrappleberryRouterIP: e.configureResult.GrappleberryRouterIP,
		}
	}
	return result, nil
}

// buildSteps creates the provisioning steps for deployment.
func (e *Executor) buildSteps() []distribution.ProvisioningStep {
	return []distribution.ProvisioningStep{
		e.buildPrepareStep(),
		e.buildInstallStep(),
		e.buildConfigureStep(),
	}
}

// buildPrepareStep creates the prepare phase step.
func (e *Executor) buildPrepareStep() distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepIDPrepare, "Prepare").
		Description("preparing cluster infrastructure").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			return e.provisioner.Prepare(ctx, e.cfg)
		}).
		MustBuild()
}

// buildInstallStep creates the install phase step.
func (e *Executor) buildInstallStep() distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepIDInstall, "Install").
		Description("installing cluster components").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			opts := install.NewOptions(e.cfg, e.projectRoot)
			return e.provisioner.Install(ctx, e.cfg, opts)
		}).
		MustBuild()
}

// buildConfigureStep creates the configure phase step.
func (e *Executor) buildConfigureStep() distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepIDConfigure, "Configure").
		Description("configuring cluster").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			result, err := e.provisioner.Configure(ctx, e.cfg)
			if err != nil {
				return err
			}
			e.configureResult = result
			return nil
		}).
		MustBuild()
}
