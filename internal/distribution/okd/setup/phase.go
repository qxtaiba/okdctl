// Package setup provides the setup phase for OKD cluster provisioning.
package setup

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/platform"
)

// Defaults for the ignition HTTP server. Port 80 is elided from generated
// URLs; port 0 falls back to DefaultIgnitionPort.
const (
	DefaultIgnitionPort = 8080
	HTTPDefaultPort     = 80
)

// openshift-install integration points. openshiftSubdir is the manifests
// subdirectory openshift-install creates beneath clusterDir; openshiftInstallBin
// is the binary name invoked for manifest and ignition generation.
const (
	openshiftSubdir     = "openshift"
	openshiftInstallBin = "openshift-install"
)

// Options configures a setup run: download and upload toggles plus an
// AutoDownloadISO switch that skips the "is the ISO present?" prompt.
type Options struct {
	phase.BaseOptions
	DownloadDir     string
	SkipDownloads   bool
	SkipISOs        bool
	SkipHAProxy     bool
	SkipFirewall    bool
	AutoDownloadISO bool
	Verbose         bool
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

// BuildIgnitionURL builds the base http:// URL where ignition payloads are
// served. Port 80 is elided from the URL; port 0 falls back to the default
// ignition port.
func BuildIgnitionURL(ip string, port int) string {
	if port == 0 {
		port = DefaultIgnitionPort
	}
	if port == HTTPDefaultPort {
		return fmt.Sprintf("http://%s/ignition", ip)
	}
	return fmt.Sprintf("http://%s:%d/ignition", ip, port)
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
	Role phase.NodeRole
	IP   string
	MAC  string
}

// Phase drives the setup flow: artifact download, config generation,
// ignition upload, and bastion service configuration.
type Phase struct {
	phase.BasePhase
	OS         platform.OS
	Pkg        platform.PackageManager
	BinDir     string
	loggedISOs map[string]bool
}

// New constructs a setup Phase with the given version tag and options.
// Host OS detection populates OS and Pkg; detection errors fall back to RHEL/dnf.
func New(version string, opts ...phase.BasePhaseOption) *Phase {
	bp := phase.NewBasePhase(version, opts...)
	detectedOS := platform.DetectOrDefault(bp.Log)
	return &Phase{
		BasePhase: bp,
		OS:        detectedOS,
		Pkg:       platform.NewPackageManager(detectedOS),
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

// PrintSetupCompletionSummary logs the cluster-config dir and terraform
// environment a user needs to reference for the follow-up install step.
func (p *Phase) PrintSetupCompletionSummary(cfg *config.Config, opts *Options) {
	clusterDir := phase.ClusterConfigDir(opts.WorkDir)
	tfEnv := phase.GetTerraformEnv(cfg)

	p.Log.Info("setup: cluster config saved", "path", clusterDir)
	p.Log.Info("setup: terraform environment set", "env", tfEnv)
}
