// Package setup provides the setup phase for OKD cluster provisioning.
package setup

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/dns"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/paths"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

const (
	DefaultIgnitionPort = 8080
	HTTPDefaultPort     = 80
)

type Options struct {
	paths.BaseOptions
	DownloadDir     string
	SkipDownloads   bool
	SkipISOs        bool
	SkipHAProxy     bool
	SkipFirewall    bool
	AutoDownloadISO bool
	Verbose         bool
}

func DefaultOptions(projectRoot string) Options {
	return Options{
		BaseOptions: paths.BaseOptions{
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
	Role string // bootstrap, master, worker
	IP   string
	MAC  string
}

type Phase struct {
	paths.BasePhase
}

func New(exec *executor.Executor, logger utils.Logger, version string) *Phase {
	return &Phase{
		BasePhase: paths.NewBasePhase(exec, logger, version),
	}
}

func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts Options) error {
	p.Log.Info("setup: starting okd cluster configuration")

	orchestrator := distribution.NewOrchestrator(
		p.newInstallPackagesStep(opts),
		p.newInstallToolsStep(cfg),
		p.newEnsureWorkDirStep(opts),
		p.newDownloadToolsStep(cfg, opts),
		p.newGenerateInstallConfigStep(cfg, opts),
		p.newGenerateManifestsStep(opts),
		p.newGenerateKubeVIPManifestsStep(cfg, opts),
		p.newInjectManifestsStep(opts),
		p.newCompactClusterManifestsStep(cfg, opts),
		p.newGenerateIgnitionStep(opts),
		p.newInstallApacheStep(cfg, opts),
		p.newDeployIgnitionStep(cfg, opts),
		p.newVerifyWebServerStep(cfg, opts),
		p.newBuildISOsStep(cfg, opts),
		p.newUploadISOsStep(cfg, opts),
		p.newGenerateTfvarsStep(cfg, opts),
		p.newConfigureHAProxyStep(cfg, opts),
		p.newConfigureFirewallStep(opts),
		p.newConfigureDNSStep(cfg, opts),
	)
	orchestrator.SetLogger(p.Log)

	if err := orchestrator.Run(ctx); err != nil {
		return err
	}

	p.Log.Info("setup: cluster configuration completed successfully")
	p.PrintSetupCompletionSummary(cfg, opts)

	return nil
}

func (p *Phase) PrintSetupCompletionSummary(cfg *config.Config, opts Options) {
	clusterDir := paths.ClusterConfigDir(opts.WorkDir)
	tfEnv := paths.GetTerraformEnv(cfg)

	p.Log.Info(fmt.Sprintf("setup: cluster config saved to %s", clusterDir))
	p.Log.Info(fmt.Sprintf("setup: terraform environment set to %s", tfEnv))
}

func (p *Phase) dnsFunctions() dnsFuncs {
	return dnsFuncs{
		setupDnsmasq: func(ctx context.Context, fallbackDNS []string) error {
			return dns.Setup(ctx, fallbackDNS, p.Log)
		},
		deployBootstrapDNS: func(ctx context.Context, cfg *config.Config) error {
			return dns.DeployBootstrap(ctx, cfg)
		},
		generateBootstrapDNSConfig: func(cfg *config.Config, outputDir string) (string, string, error) {
			return dns.GenerateBootstrapConfig(cfg, outputDir)
		},
	}
}

type dnsFuncs struct {
	setupDnsmasq               func(ctx context.Context, fallbackDNS []string) error
	deployBootstrapDNS         func(ctx context.Context, cfg *config.Config) error
	generateBootstrapDNSConfig func(cfg *config.Config, outputDir string) (string, string, error)
}
