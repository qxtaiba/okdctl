package deployment

import (
	"context"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/install"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

const (
	StepIDPrepare   distribution.StepID = "prepare"
	StepIDInstall   distribution.StepID = "install"
	StepIDConfigure distribution.StepID = "configure"
)

type Executor struct {
	cfg             *config.Config
	provisioner     *okd.Provisioner
	projectRoot     string
	logger          utils.Logger
	configureResult *postinstall.Result
}

type ExecutorOption func(*Executor)

func WithLogger(l utils.Logger) ExecutorOption {
	return func(e *Executor) {
		if l != nil {
			e.logger = l
		}
	}
}

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
		logger:      utils.NoopLogger(),
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

func (e *Executor) Execute(ctx context.Context) (*Result, error) {
	steps := e.buildSteps()
	orchestrator := distribution.NewOrchestrator(steps...)
	orchestrator.SetLogger(e.logger)

	if err := orchestrator.Run(ctx); err != nil {
		return nil, err
	}

	var result *Result
	if e.configureResult != nil {
		result = &Result{
			KubeVipIP:        e.configureResult.KubeVipIP,
			BastionIP:        e.configureResult.BastionIP,
			BootstrapCleaned: e.configureResult.BootstrapCleaned,
			DNSDeployed:      e.configureResult.DNSDeployed,
		}
	}
	return result, nil
}

func (e *Executor) buildSteps() []distribution.ProvisioningStep {
	return []distribution.ProvisioningStep{
		e.buildPrepareStep(),
		e.buildInstallStep(),
		e.buildConfigureStep(),
	}
}

func (e *Executor) buildPrepareStep() distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepIDPrepare, "Prepare").
		Description("preparing cluster infrastructure").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			return e.provisioner.Prepare(ctx, e.cfg)
		}).
		MustBuild()
}

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
