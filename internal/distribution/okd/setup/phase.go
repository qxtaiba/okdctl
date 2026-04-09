// Package setup provides the setup phase for OKD cluster provisioning.
package setup

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/dns"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/phase"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

const (
	DefaultIgnitionPort = 8080
	HTTPDefaultPort     = 80
)

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
	Role string // bootstrap, master, worker
	IP   string
	MAC  string
}

type Phase struct {
	phase.BasePhase
}

func New(exec *executor.Executor, logger *slog.Logger, version string) *Phase {
	return &Phase{
		BasePhase: phase.NewBasePhase(exec, logger, version),
	}
}

func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts Options) error {
	p.Log.Info("setup: starting okd cluster configuration")

	// Preflight: many setup steps call sudo. If passwordless sudo is not
	// configured and the user has no cached timestamp, the first sudo call
	// will stall waiting for a password read from stdin, which looks like a
	// hung deployment. Warn once up front rather than blocking; the user may
	// have primed sudo earlier in this session.
	if err := system.HasPasswordlessSudo(ctx); err != nil {
		p.Log.Warn("setup: passwordless sudo not configured; next sudo command may hang waiting for password")
	}

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
	clusterDir := phase.ClusterConfigDir(opts.WorkDir)
	tfEnv := phase.GetTerraformEnv(cfg)

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
