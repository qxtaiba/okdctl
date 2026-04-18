// Package setup provides the setup phase for OKD cluster provisioning.
package setup

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/platform"
)

const (
	DefaultIgnitionPort = 8080
	HTTPDefaultPort     = 80
)

type Options struct {
	phase.BaseOptions
	DownloadDir       string
	SkipDownloads     bool
	SkipISOs          bool
	SkipHAProxy       bool
	SkipFirewall      bool
	AutoDownloadISO   bool
	Verbose           bool
	OKDReleaseBaseURL string
}

func DefaultOptions(projectRoot string) Options {
	return Options{
		BaseOptions: phase.BaseOptions{
			WorkDir:     filepath.Join(projectRoot, "okd-install"),
			ProjectRoot: projectRoot,
		},
		DownloadDir: filepath.Join(projectRoot, "okd-install", "downloads"),
	}
}

func BuildIgnitionURL(ip string, port int) string {
	if port == 0 {
		port = DefaultIgnitionPort
	}
	if port == HTTPDefaultPort {
		return fmt.Sprintf("http://%s/ignition", ip)
	}
	return fmt.Sprintf("http://%s:%d/ignition", ip, port)
}

type CoreOSInfo struct {
	Version      string
	ISOUrl       string
	ISOChecksum  string
	Architecture string
}

type NodeInfo struct {
	Name string
	Role phase.NodeRole
	IP   string
	MAC  string
}

type Phase struct {
	phase.BasePhase
	OS  platform.OS
	Pkg platform.PackageManager
}

func New(exec *executor.Executor, logger *slog.Logger, version string) *Phase {
	detectedOS, err := platform.Detect()
	if err != nil {
		if logger != nil {
			logger.Warn(fmt.Sprintf("platform: %v", err))
		}
		detectedOS = platform.OS{Family: "rhel", ID: "unknown", Version: ""}
	}
	return &Phase{
		BasePhase: phase.NewBasePhase(exec, logger, version),
		OS:        detectedOS,
		Pkg:       platform.NewPackageManager(detectedOS),
	}
}

func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts *Options) error {
	p.Log.Info("setup: starting okd cluster configuration")

	orchestrator := distribution.NewOrchestrator(distribution.BuildSteps(p.setupSteps(cfg, opts))...)
	orchestrator.SetLogger(p.Log)

	if err := orchestrator.Run(ctx); err != nil {
		return err
	}

	p.Log.Info("setup: cluster configuration completed successfully")
	p.PrintSetupCompletionSummary(cfg, opts)

	return nil
}

func (p *Phase) PrintSetupCompletionSummary(cfg *config.Config, opts *Options) {
	clusterDir := phase.ClusterConfigDir(opts.WorkDir)
	tfEnv := phase.GetTerraformEnv(cfg)

	p.Log.Info(fmt.Sprintf("setup: cluster config saved to %s", clusterDir))
	p.Log.Info(fmt.Sprintf("setup: terraform environment set to %s", tfEnv))
}
