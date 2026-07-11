// Package setup runs the OKD setup phase: install host packages and the
// tool trio (oc, openshift-install, terraform), render install-config and
// Kubernetes manifests (including kube-vip), generate ignition files, build
// custom CoreOS ISOs with embedded kargs, and configure HAProxy, dnsmasq,
// and the bastion firewall. Steps are declared in setupBaseSteps,
// setupManifestSteps, setupWebSteps, and setupInfraSteps, concatenated by
// setupSteps. Exported builders (BuildLiveKargs, BuildDestKargs,
// ExtractNetworkConfig, EnsureIgnitionCert) are the package's public API
// surface even though today's only callers are intra-package.
package setup

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/platform"
)

// Defaults for the ignition HTTPS server. Port 0 in BuildIgnitionURL falls
// back to DefaultIgnitionHTTPSPort. DefaultIgnitionPort is kept for back-
// compat with callers that still reference it.
const (
	DefaultIgnitionPort      = 8080
	DefaultIgnitionHTTPSPort = 443
	HTTPDefaultPort          = 80
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
		BaseOptions: phase.BaseOptions{
			WorkDir:      filepath.Join(projectRoot, "okd-install"),
			ProjectRoot:  projectRoot,
			TerraformEnv: phase.GetTerraformEnv(cfg),
		},
		DownloadDir: filepath.Join(projectRoot, "okd-install", "downloads"),
	}
}

// BuildIgnitionURL builds the base https:// URL where ignition payloads are
// served. Port 0 falls back to DefaultIgnitionHTTPSPort; port 443 is elided
// per RFC 7230.
func BuildIgnitionURL(ip string, port int) string {
	if port == 0 {
		port = DefaultIgnitionHTTPSPort
	}
	if port == DefaultIgnitionHTTPSPort {
		return fmt.Sprintf("https://%s/ignition", ip)
	}
	return fmt.Sprintf("https://%s:%d/ignition", ip, port)
}

// CoreOSInfo describes a Fedora CoreOS download candidate resolved from
// the CoreOS stream metadata.
type CoreOSInfo struct {
	Version      string
	ISOUrl       string
	ISOChecksum  string
	Architecture string
}

// NodeInfo identifies a single VM the setup phase emits into the generated
// Terraform tfvars (role, IP, MAC).
type NodeInfo struct {
	Name string
	Role nodetypes.NodeRole
	IP   string
	MAC  string
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
	phase.BasePhase
	OS         platform.OS
	Pkg        packageInstaller
	BinDir     string
	loggedISOs map[string]bool
}

// New constructs a setup Phase with the given options. Host OS detection
// populates OS and Pkg; detection errors fall back to RHEL/dnf.
func New(opts ...phase.BasePhaseOption) *Phase {
	bp := phase.NewBasePhase(opts...)
	bp.Log = bp.Log.With("phase", "setup")
	detectedOS := platform.DetectOrDefault(bp.Log)
	return &Phase{
		BasePhase: bp,
		OS:        detectedOS,
		Pkg:       platform.NewPackageManager(detectedOS, bp.Log),
	}
}

// Execute runs the setup phase step sequence and returns each step's
// result. A non-nil error means orchestration stopped early.
func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts *Options) ([]distribution.StepResult, error) {
	p.Log.Info("setup: starting okd cluster configuration")

	orchestrator := distribution.NewOrchestrator(distribution.BuildSteps(p.setupSteps(cfg, opts))...)
	orchestrator.SetLogger(p.Log)
	orchestrator.SetMetricsRecorder(p.Recorder)

	if err := orchestrator.Run(ctx); err != nil {
		return orchestrator.Results(), err
	}

	p.Log.Info("setup: cluster configuration completed successfully")
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
	clusterDir := phase.ClusterConfigDir(opts.WorkDir)
	tfEnv := phase.GetTerraformEnv(cfg)

	p.Log.Info("setup: cluster config saved", "path", clusterDir)
	p.Log.Info("setup: terraform environment set", "env", tfEnv)
}
