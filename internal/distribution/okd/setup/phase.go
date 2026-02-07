// Package setup provides the setup phase implementation for OKD cluster provisioning.
// It handles downloading tools, generating configuration files, building ISOs,
// and configuring infrastructure services (HAProxy, Apache, dnsmasq).
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
	"github.com/qxtaiba/okd-proxmox-cli/internal/logging"
)

// Default ports for ignition server.
const (
	DefaultIgnitionPort = 8080
	HTTPDefaultPort     = 80
)

// Options configures the setup phase.
type Options struct {
	paths.BaseOptions

	// DownloadDir is where to download tools.
	DownloadDir string

	// SkipDownloads skips downloading OKD tools if they exist.
	SkipDownloads bool

	// SkipISOs skips building custom ISOs.
	SkipISOs bool

	// SkipHAProxy skips HAProxy configuration.
	SkipHAProxy bool

	// SkipFirewall skips firewall configuration.
	SkipFirewall bool

	// AutoDownloadISO automatically downloads CoreOS ISO if missing.
	AutoDownloadISO bool

	// Verbose enables extra logging.
	Verbose bool
}

// DefaultOptions returns default setup options.
func DefaultOptions(projectRoot string) Options {
	return Options{
		BaseOptions: paths.BaseOptions{
			WorkDir:     filepath.Join(projectRoot, "okd-install"),
			ProjectRoot: projectRoot,
		},
		DownloadDir: filepath.Join(projectRoot, "okd-install", "downloads"),
	}
}

// BuildIgnitionURL constructs the ignition URL from IP and port.
func BuildIgnitionURL(ip string, port int) string {
	if port == 0 {
		port = DefaultIgnitionPort
	}
	if port == HTTPDefaultPort {
		return fmt.Sprintf("http://%s/ignition", ip)
	}
	return fmt.Sprintf("http://%s:%d/ignition", ip, port)
}

// CoreOSInfo holds information about the CoreOS release.
type CoreOSInfo struct {
	Version      string
	ISOUrl       string
	ISOChecksum  string
	Architecture string
}

// NodeInfo holds information about a cluster node.
type NodeInfo struct {
	Name string
	Role string // bootstrap, master, worker
	IP   string
	MAC  string
}

// Phase coordinates the setup phase execution.
type Phase struct {
	paths.BasePhase
}

// New creates a new setup phase coordinator.
func New(exec *executor.Executor, logger logging.Logger, version string) *Phase {
	return &Phase{
		BasePhase: paths.NewBasePhase(exec, logger, version),
	}
}

// Execute runs the complete setup phase.
func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts Options) error {
	p.LogInfo("setup: starting okd cluster configuration")

	orchestrator := distribution.NewOrchestrator(
		p.newInstallPackagesStep(opts),
		p.newInstallToolsStep(cfg),
		p.newEnsureWorkDirStep(opts),
		p.newDownloadToolsStep(cfg, opts),
		p.newGenerateInstallConfigStep(cfg, opts),
		p.newGenerateManifestsStep(opts),
		p.newInjectManifestsStep(opts),
		p.newGenerateIgnitionStep(opts),
		p.newInstallApacheStep(cfg, opts),
		p.newDeployIgnitionStep(cfg, opts),
		p.newVerifyWebServerStep(cfg, opts),
		p.newBuildISOsStep(cfg, opts),
		p.newUploadISOsStep(cfg, opts),
		p.newGenerateTfvarsStep(cfg, opts),
		p.newConfigureHAProxyStep(cfg, opts),
		p.newConfigureBastionVIPStep(cfg, opts), // Assign VIP to bastion for bootstrap
		p.newConfigureFirewallStep(opts),
		p.newConfigureDNSStep(cfg, opts),
	)
	orchestrator.SetLogger(p.Log)

	if err := orchestrator.Run(ctx); err != nil {
		return err
	}

	p.LogInfo("setup: cluster configuration completed successfully")
	p.PrintSetupCompletionSummary(cfg, opts)

	return nil
}

// PrintSetupCompletionSummary prints a brief summary of generated files.
func (p *Phase) PrintSetupCompletionSummary(cfg *config.Config, opts Options) {
	clusterDir := paths.ClusterConfigDir(opts.WorkDir)
	tfEnv := paths.GetTerraformEnv(cfg)

	p.Log.Info(fmt.Sprintf("setup: cluster config saved to %s", clusterDir))
	p.Log.Info(fmt.Sprintf("setup: terraform environment set to %s", tfEnv))
}

// dnsFunctions returns DNS function implementations for the DNS step.
func (p *Phase) dnsFunctions() dnsFuncs {
	return dnsFuncs{
		setupDnsmasq: dns.Setup,
		deployBootstrapDNS: func(ctx context.Context, cfg *config.Config) error {
			return dns.DeployBootstrap(ctx, cfg)
		},
		generateBootstrapDNSConfig: func(cfg *config.Config, outputDir string) (string, string, error) {
			return dns.GenerateBootstrapConfig(cfg, outputDir)
		},
	}
}

// dnsFuncs holds DNS function implementations.
type dnsFuncs struct {
	setupDnsmasq               func(ctx context.Context, fallbackDNS []string) error
	deployBootstrapDNS         func(ctx context.Context, cfg *config.Config) error
	generateBootstrapDNSConfig func(cfg *config.Config, outputDir string) (string, string, error)
}
