// Package setup runs the OKD setup phase: install host packages and the
// tool trio (oc, openshift-install, terraform), render install-config and
// Kubernetes manifests (including kube-vip, chrony, and fstrim), generate
// ignition files, and configure HAProxy, dnsmasq, and the bastion firewall.
// The ISO build/upload and ignition-server machinery shared with day-2 node
// operations lives in the embedded provision.Provisioner. Steps are declared
// in setupBaseSteps, setupManifestSteps, setupWebSteps, and setupInfraSteps,
// concatenated by setupSteps.
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

// openshift-install integration points. openshiftSubdir is the manifests
// subdirectory openshift-install creates beneath clusterDir; openshiftInstallBin
// is the binary name invoked for manifest and ignition generation.
const (
	openshiftSubdir     = "openshift"
	openshiftInstallBin = "openshift-install"
	ocBin               = "oc"
	kubectlBin          = "kubectl"
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

// provisionOpts projects the setup options onto the narrow option set the
// shared provisioning machinery consumes.
func (o *Options) provisionOpts() provision.Options {
	return provision.Options{ProjectRoot: o.ProjectRoot, WorkDir: o.WorkDir}
}

// packageInstaller is the subset of platform.Manager the setup phase calls.
// A consumer-side interface so tests can substitute a no-op fake without
// shelling out to the real host package manager.
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

// New constructs a setup Phase with the given options. Host OS detection
// populates OS and Pkg; detection errors fall back to RHEL/dnf.
func New(opts ...phase.BasePhaseOption) *Phase {
	bp := phase.NewBasePhase(opts...)
	bp.Log = bp.Log.With("phase", "setup")
	detectedOS := platform.DetectOrDefault(bp.Log)
	return &Phase{
		Provisioner: provision.Provisioner{BasePhase: bp, OS: detectedOS},
		Pkg:         platform.NewPackageManager(detectedOS, bp.Log),
	}
}

// Execute runs the setup phase step sequence and returns each step's
// result. A non-nil error means orchestration stopped early. cfg must be
// the same cfg passed to NewOptions — opts was derived from it and the two
// are not re-validated for consistency here.
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

// StepDefs returns the ordered step definitions this phase executes for
// cfg/opts, without running them. Provisioner.DeploySteps calls this for
// the deploy --dry-run listing, so the listing cannot drift from Execute.
func (p *Phase) StepDefs(cfg *config.Config, opts *Options) []distribution.StepDef {
	return p.setupSteps(cfg, opts)
}

// PrintSetupCompletionSummary logs the cluster-config dir and terraform
// environment a user needs to reference for the follow-up install step.
func (p *Phase) PrintSetupCompletionSummary(cfg *config.Config, opts *Options) {
	clusterDir := workspace.ClusterConfigDir(opts.WorkDir)
	tfEnv := cfg.TerraformEnvName()

	p.Log.Info("setup: cluster config saved", "path", clusterDir)
	p.Log.Info("setup: terraform environment set", "env", tfEnv)
}
