// Package setup runs the OKD setup phase: install tools, render
// install-config/manifests/ignition, and configure HAProxy, dnsmasq, and
// the firewall. ISO/ignition-server machinery shared with day-2 node ops
// lives in the embedded provision.Provisioner.
package setup

import (
	"context"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/provision"
	"github.com/qxtaiba/okdctl/internal/platform"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// openshiftSubdir is the manifests subdir openshift-install creates under
// clusterDir; openshiftInstallBin is its binary name.
const (
	openshiftSubdir     = "openshift"
	openshiftInstallBin = "openshift-install"
)

// Options configures a setup run: download and upload toggles.
type Options struct {
	phase.BaseOptions
	DownloadDir   string
	SkipDownloads bool
	SkipISOs      bool
	SkipHAProxy   bool
	SkipFirewall  bool
}

// NewOptions returns setup Options with WorkDir/DownloadDir rooted at
// projectRoot and TerraformEnv resolved from cfg.
func NewOptions(cfg *config.Config, projectRoot string) Options {
	return Options{
		BaseOptions: phase.NewBaseOptions(cfg, projectRoot),
		DownloadDir: filepath.Join(workspace.WorkDir(projectRoot), "downloads"),
	}
}

func (o *Options) provisionOpts() provision.Options {
	return provision.Options{ProjectRoot: o.ProjectRoot, WorkDir: o.WorkDir}
}

// packageInstaller is the subset of platform.Manager used here, defined
// consumer-side so tests can fake it.
type packageInstaller interface {
	Install(ctx context.Context, packages []string) error
}

// Phase drives the setup flow: artifact download, config generation,
// ignition upload, and bastion service configuration.
type Phase struct {
	provision.Provisioner
	Pkg    packageInstaller
	BinDir string
}

// New constructs a setup Phase with the given options; host-OS detection
// populates OS and Pkg, falling back to RHEL/dnf on error.
func New(opts ...phase.BasePhaseOption) *Phase {
	bp := phase.NewBasePhase(opts...)
	bp.Log = bp.Log.With("phase", "setup")
	detectedOS := platform.DetectOrDefault(bp.Log)
	return &Phase{
		Provisioner: provision.Provisioner{BasePhase: bp, OS: detectedOS},
		Pkg:         platform.NewPackageManager(detectedOS, bp.Log),
	}
}

// Execute runs the setup phase steps, stopping at the first error. cfg must be
// the same value passed to NewOptions, since opts is derived from it and not
// re-checked here.
func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts *Options) ([]distribution.StepResult, error) {
	p.Log.Info("setup: starting okd cluster configuration")

	orchestrator := distribution.NewOrchestrator(distribution.BuildSteps(p.setupSteps(cfg, opts))...)
	orchestrator.SetLogger(p.Log)
	orchestrator.SetMetricsRecorder(p.Recorder)

	if err := orchestrator.Run(ctx); err != nil {
		return orchestrator.Results(), err
	}

	p.Log.Info("setup: cluster configuration completed")
	p.PrintSetupCompletionSummary(cfg, opts)

	return orchestrator.Results(), nil
}

// StepDefs returns cfg/opts's ordered step definitions without running them;
// Provisioner.DeploySteps calls this for --dry-run so it can't drift from
// Execute.
func (p *Phase) StepDefs(cfg *config.Config, opts *Options) []distribution.StepDef {
	return p.setupSteps(cfg, opts)
}

// PrintSetupCompletionSummary logs the cluster-config dir and terraform
// environment needed for the follow-up install step.
func (p *Phase) PrintSetupCompletionSummary(cfg *config.Config, opts *Options) {
	clusterDir := workspace.ClusterConfigDir(opts.WorkDir)
	tfEnv := cfg.TerraformEnvName()

	p.Log.Info("setup: cluster config saved", "path", clusterDir)
	p.Log.Info("setup: terraform environment set", "env", tfEnv)
}
