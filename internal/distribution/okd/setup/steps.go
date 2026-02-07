package setup

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/paths"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/templates"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// Step IDs for setup operations.
const (
	StepInstallPackages   distribution.StepID = "install-packages"
	StepInstallTools      distribution.StepID = "install-tools"
	StepEnsureWorkDir     distribution.StepID = "ensure-workdir"
	StepDownloadTools     distribution.StepID = "download-tools"
	StepGenerateConfig    distribution.StepID = "generate-config"
	StepGenerateManifests     distribution.StepID = "generate-manifests"
	StepGenerateKubeVIP       distribution.StepID = "generate-kubevip-manifests"
	StepInjectManifests       distribution.StepID = "inject-manifests"
	StepCompactCluster        distribution.StepID = "compact-cluster-manifests"
	StepGenerateIgnition  distribution.StepID = "generate-ignition"
	StepInstallApache     distribution.StepID = "install-apache"
	StepDeployIgnition    distribution.StepID = "deploy-ignition"
	StepVerifyWebServer   distribution.StepID = "verify-webserver"
	StepBuildISOs         distribution.StepID = "build-isos"
	StepUploadISOs        distribution.StepID = "upload-isos"
	StepGenerateTfvars    distribution.StepID = "generate-tfvars"
	StepConfigureHAProxy  distribution.StepID = "configure-haproxy"
	StepConfigureBastionVIP distribution.StepID = "configure-bastion-vip"
	StepConfigureFirewall distribution.StepID = "configure-firewall"
	StepConfigureDNS      distribution.StepID = "configure-dns"
)

// ═══════════════════════════════════════════════════════════════════════════════
// INSTALL PACKAGES STEP
// ═══════════════════════════════════════════════════════════════════════════════

// systemPackages returns the list of packages to install via dnf.
func systemPackages() []string {
	return []string{
		"coreos-installer",
		"haproxy",
		"httpd",
		"dnsmasq",
		"iputils", // provides arping for gratuitous ARP announcements
	}
}

func (p *Phase) newInstallPackagesStep(opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepInstallPackages, "Install System Packages").
		Description("installing required system packages").
		Fatal(false). // Non-fatal since services can be configured manually
		Execute(func(ctx context.Context) error {
			packages := systemPackages()

			var packagesToInstall []string
			for _, pkg := range packages {
				cmdName := pkg
				if !executor.CommandExists(cmdName) {
					packagesToInstall = append(packagesToInstall, pkg)
					p.LogInfo(fmt.Sprintf("packages: %s not found", pkg))
				} else {
					p.LogInfo(fmt.Sprintf("packages: %s already installed", pkg))
				}
			}

			if len(packagesToInstall) == 0 {
				p.LogInfo("packages: all system packages already installed")
				return nil
			}

			p.LogInfo(fmt.Sprintf("packages: installing %d missing package(s) via dnf", len(packagesToInstall)))
			if err := system.InstallPackages(ctx, packagesToInstall, "system dependencies"); err != nil {
				p.LogWarn(fmt.Sprintf("packages: installation had warnings: %v", err))
			}

			return nil
		}).
		OnError(func(err error) {
			p.LogWarn(fmt.Sprintf("packages: system installation had warnings: %v", err))
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// INSTALL EXTERNAL TOOLS STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newInstallToolsStep(cfg *config.Config) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepInstallTools, "Install External Tools").
		Description("installing core tools and addon-required tools").
		Fatal(false). // Non-fatal since tools may already be installed or user can install manually
		Execute(func(ctx context.Context) error {
			return p.InstallExternalTools(ctx, cfg)
		}).
		OnError(func(err error) {
			p.LogWarn(fmt.Sprintf("tools: external installation had warnings: %v", err))
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// ENSURE WORKDIR STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newEnsureWorkDirStep(opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepEnsureWorkDir, "Ensure Work Directory").
		Description("creating work directory").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			return system.EnsureDir(opts.WorkDir)
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// DOWNLOAD TOOLS STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newDownloadToolsStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepDownloadTools, "Download OKD Tools").
		Description(fmt.Sprintf("downloading OKD tools version %s", cfg.Distribution.Version)).
		Fatal(true).
		SkipWhen(func() bool { return opts.SkipDownloads }).
		SkipReason("downloads disabled").
		Execute(func(ctx context.Context) error {
			if err := p.DownloadOKDTools(ctx, cfg.Distribution.Version, opts); err != nil {
				return utils.WrapError("failed to download OKD tools", err)
			}
			p.LogInfo("tools: sha256 checksums validated successfully")
			return nil
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// GENERATE INSTALL CONFIG STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newGenerateInstallConfigStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	clusterDir := paths.ClusterConfigDir(opts.WorkDir)

	return distribution.NewStepBuilder(StepGenerateConfig, "Generate Install Config").
		Description("generating install-config.yaml").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			if err := p.GenerateInstallConfig(ctx, cfg, clusterDir); err != nil {
				return utils.WrapError("failed to generate install-config", err)
			}
			p.LogInfo(fmt.Sprintf("config: install-config.yaml generated with %d masters and %d workers",
				cfg.Topology.ControlPlane.Count, cfg.Topology.Workers.Count))
			return nil
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// GENERATE MANIFESTS STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newGenerateManifestsStep(opts Options) distribution.ProvisioningStep {
	clusterDir := paths.ClusterConfigDir(opts.WorkDir)

	return distribution.NewStepBuilder(StepGenerateManifests, "Generate Manifests").
		Description("generating kubernetes manifests").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			if err := p.GenerateManifests(ctx, clusterDir); err != nil {
				return utils.WrapError("failed to generate manifests", err)
			}
			p.LogInfo("manifests: kubernetes manifests generated")
			return nil
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// GENERATE KUBE-VIP MANIFESTS STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newGenerateKubeVIPManifestsStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	clusterDir := paths.ClusterConfigDir(opts.WorkDir)

	return distribution.NewStepBuilder(StepGenerateKubeVIP, "Generate Kube-VIP Manifests").
		Description("generating kube-vip RBAC and DaemonSet manifests for VIP management").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			vip := netutil.DeriveVIPFromStaticIP(cfg.Networking.StaticIP.Start)
			if vip == "" {
				return fmt.Errorf("failed to derive VIP from static IP start: %s", cfg.Networking.StaticIP.Start)
			}

			iface := cfg.Networking.StaticIP.Interface
			if iface == "" {
				iface = "ens18" // default virtio interface on Proxmox VMs
			}

			openshiftDir := filepath.Join(clusterDir, "openshift")
			if err := system.EnsureDir(openshiftDir); err != nil {
				return utils.WrapError("failed to ensure openshift manifests directory", err)
			}

			// Render and write RBAC manifests (one file per resource)
			rbacManifests, err := templates.RenderKubeVIPRBACManifests()
			if err != nil {
				return utils.WrapError("failed to render kube-vip RBAC manifests", err)
			}
			for _, m := range rbacManifests {
				path := filepath.Join(openshiftDir, m.Filename)
				if err := system.AtomicWriteString(path, m.Content, 0644); err != nil {
					return utils.WrapErrorf(err, "failed to write %s", m.Filename)
				}
			}

			// Render and write DaemonSet manifest
			ds, err := templates.RenderKubeVIPDaemonSet(templates.KubeVIPData{
				VIPAddress: vip,
				Interface:  iface,
			})
			if err != nil {
				return utils.WrapError("failed to render kube-vip DaemonSet manifest", err)
			}
			dsPath := filepath.Join(openshiftDir, "99-kube-vip-daemonset.yaml")
			if err := system.AtomicWriteString(dsPath, ds, 0644); err != nil {
				return utils.WrapError("failed to write kube-vip DaemonSet manifest", err)
			}

			p.LogInfo(fmt.Sprintf("kubevip: manifests generated (vip=%s, interface=%s, image=ghcr.io/kube-vip/kube-vip:%s)",
				vip, iface, templates.DefaultKubeVIPImageTag))
			return nil
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// INJECT MANIFESTS STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newInjectManifestsStep(opts Options) distribution.ProvisioningStep {
	clusterDir := paths.ClusterConfigDir(opts.WorkDir)

	return distribution.NewStepBuilder(StepInjectManifests, "Inject Custom Manifests").
		Description("injecting custom manifests").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			count, err := p.InjectCustomManifests(ctx, opts.ProjectRoot, clusterDir)
			if err != nil {
				return utils.WrapError("failed to inject custom manifests", err)
			}
			if count > 0 {
				p.LogInfo(fmt.Sprintf("manifests: injected %d custom manifest(s) from automation/config/manifests", count))
			}
			return nil
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// COMPACT CLUSTER MANIFESTS STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newCompactClusterManifestsStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	clusterDir := paths.ClusterConfigDir(opts.WorkDir)

	return distribution.NewStepBuilder(StepCompactCluster, "Inject Compact Cluster Manifests").
		Description("injecting ingress controller placement for compact cluster").
		Fatal(true).
		SkipWhen(func() bool { return cfg.Topology.Workers.Count > 0 }).
		SkipReason("cluster has workers").
		Execute(func(ctx context.Context) error {
			if err := p.InjectCompactClusterManifests(ctx, clusterDir, cfg.Topology.Workers.Count, cfg.Topology.ControlPlane.Count); err != nil {
				return utils.WrapError("failed to inject compact cluster manifests", err)
			}
			p.LogInfo("manifests: injected ingress controller master placement for compact cluster")
			return nil
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// GENERATE IGNITION STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newGenerateIgnitionStep(opts Options) distribution.ProvisioningStep {
	clusterDir := paths.ClusterConfigDir(opts.WorkDir)

	return distribution.NewStepBuilder(StepGenerateIgnition, "Generate Ignition").
		Description("generating ignition files").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			if err := p.GenerateIgnitionConfigs(ctx, clusterDir); err != nil {
				return utils.WrapError("failed to generate ignition configs", err)
			}
			p.LogInfo("ignition: configurations generated and validated")
			return nil
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// INSTALL APACHE STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newInstallApacheStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepInstallApache, "Install Apache").
		Description("installing and configuring apache web server").
		Fatal(false). // Non-fatal
		Execute(func(ctx context.Context) error {
			return p.ConfigureApache(ctx, cfg)
		}).
		OnError(func(err error) {
			p.LogWarn(fmt.Sprintf("apache: installation skipped: %v", err))
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// DEPLOY IGNITION STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newDeployIgnitionStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	clusterDir := paths.ClusterConfigDir(opts.WorkDir)

	return distribution.NewStepBuilder(StepDeployIgnition, "Deploy Ignition").
		Description("deploying ignition files to apache web server").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			if err := p.DeployToWebServer(ctx, cfg, clusterDir); err != nil {
				return utils.WrapError("failed to deploy to web server", err)
			}

			webURL := BuildIgnitionURL(cfg.HTTPServer.IgnitionServerIP, cfg.HTTPServer.Port)
			p.LogInfo(fmt.Sprintf("ignition: deployed to web server at %s", webURL))
			return nil
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// VERIFY WEB SERVER STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newVerifyWebServerStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepVerifyWebServer, "Verify Web Server").
		Description("verifying web server accessibility").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			webURL := BuildIgnitionURL(cfg.HTTPServer.IgnitionServerIP, cfg.HTTPServer.Port)
			return p.VerifyWebServer(ctx, webURL)
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// BUILD ISOS STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newBuildISOsStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepBuildISOs, "Build ISOs").
		Description("building custom CoreOS ISOs").
		Fatal(true).
		SkipWhen(func() bool { return opts.SkipISOs }).
		SkipReason("ISO building disabled").
		Execute(func(ctx context.Context) error {
			return p.BuildCustomISOs(ctx, cfg, opts)
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// UPLOAD ISOS STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newUploadISOsStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepUploadISOs, "Upload ISOs").
		Description("uploading ISOs to Proxmox storage").
		Fatal(false). // Non-fatal
		SkipWhen(func() bool { return opts.SkipISOs }).
		SkipReason("ISO building disabled").
		Execute(func(ctx context.Context) error {
			if err := p.UploadCustomISOsToProxmox(ctx, cfg, opts); err != nil {
				return err
			}
			p.LogInfo("iso: all custom isos uploaded to proxmox storage")
			return nil
		}).
		OnError(func(err error) {
			p.LogWarn(fmt.Sprintf("iso: upload failed: %v", err))
			p.LogWarn("iso: you may need to upload isos manually before deploying")
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// GENERATE TFVARS STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newGenerateTfvarsStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepGenerateTfvars, "Generate Terraform Variables").
		Description("generating terraform variables").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			if err := p.GenerateTerraformVars(cfg, opts); err != nil {
				return utils.WrapError("failed to generate Terraform variables", err)
			}
			tfvarsPath := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", paths.GetTerraformEnv(cfg), "terraform.tfvars")
			p.LogInfo(fmt.Sprintf("terraform: configuration written to %s", tfvarsPath))
			return nil
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// CONFIGURE HAPROXY STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newConfigureHAProxyStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepConfigureHAProxy, "Configure HAProxy").
		Description("configuring haproxy load balancer").
		Fatal(true).
		SkipWhen(func() bool { return opts.SkipHAProxy }).
		SkipReason("HAProxy configuration disabled").
		Execute(func(ctx context.Context) error {
			if err := p.ConfigureHAProxy(ctx, cfg, opts); err != nil {
				return utils.WrapError("failed to configure HAProxy", err)
			}
			if err := p.VerifyHAProxyPorts(ctx); err != nil {
				p.LogWarn(fmt.Sprintf("haproxy: port verification warning: %v", err))
			}
			return nil
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// CONFIGURE FIREWALL STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newConfigureFirewallStep(opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepConfigureFirewall, "Configure Firewall").
		Description("configuring firewall rules for OKD").
		Fatal(true).
		SkipWhen(func() bool { return opts.SkipFirewall }).
		SkipReason("firewall configuration disabled").
		Execute(func(ctx context.Context) error {
			if err := system.ConfigureOKDFirewall(ctx, true); err != nil {
				return err
			}
			p.LogInfo("firewall: okd rules added to firewalld")
			return nil
		}).
		OnError(func(err error) {
			p.LogWarn(fmt.Sprintf("firewall: configuration failed: %v", err))
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// CONFIGURE DNS STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newConfigureDNSStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	outputDir := filepath.Join(opts.WorkDir, "dns")
	funcs := p.dnsFunctions()

	return distribution.NewStepBuilder(StepConfigureDNS, "Configure DNS").
		Description("configuring dnsmasq and deploying bootstrap dns configuration").
		Fatal(false). // Non-fatal since DNS can be configured manually
		Execute(func(ctx context.Context) error {
			p.LogInfo("dns: configuring dnsmasq service")
			if funcs.setupDnsmasq != nil {
				if err := funcs.setupDnsmasq(ctx, cfg.Networking.DNS); err != nil {
					return utils.WrapError("failed to setup dnsmasq", err)
				}
			}

			p.LogInfo("dns: deploying bootstrap dns configuration")
			if funcs.deployBootstrapDNS != nil {
				if err := funcs.deployBootstrapDNS(ctx, cfg); err != nil {
					return utils.WrapError("failed to deploy bootstrap dns", err)
				}
			}

			// Save a copy to the work directory for reference (non-fatal)
			if funcs.generateBootstrapDNSConfig != nil {
				if _, _, err := funcs.generateBootstrapDNSConfig(cfg, outputDir); err != nil {
					p.LogWarn(fmt.Sprintf("dns: failed to save config copy: %v", err))
				}
			}

			configPath := system.DnsmasqConfigPath(fmt.Sprintf("okd-%s", cfg.Cluster.Name))
			p.LogInfo(fmt.Sprintf("dns: dnsmasq configured at %s", configPath))
			return nil
		}).
		OnError(func(err error) {
			p.LogWarn(fmt.Sprintf("dns: configuration failed: %v", err))
			p.LogWarn("dns: you may need to configure dns manually")
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// CONFIGURE BASTION VIP STEP
// ═══════════════════════════════════════════════════════════════════════════════

// newConfigureBastionVIPStep creates a step that assigns the kube-vip VIP to the
// bastion's network interface. This allows the bastion to hold the VIP during
// bootstrap. When kube-vip starts on the control plane nodes, it will take over
// the VIP via gratuitous ARP announcement.
func (p *Phase) newConfigureBastionVIPStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepConfigureBastionVIP, "Configure Bastion VIP").
		Description("assigning kube-vip VIP to bastion interface for bootstrap").
		Fatal(true). // VIP is required - DNS points api.* to the VIP, so bootstrap fails without it
		SkipWhen(func() bool { return opts.SkipHAProxy }). // Skip if HAProxy is skipped (no load balancing)
		SkipReason("HAProxy disabled - VIP not needed on bastion").
		Execute(func(ctx context.Context) error {
			vip := netutil.DeriveVIPFromStaticIP(cfg.Networking.StaticIP.Start)
			if vip == "" {
				return fmt.Errorf("failed to derive VIP from static IP start: %s", cfg.Networking.StaticIP.Start)
			}

			// Detect the bastion's actual interface rather than using cfg.Networking.StaticIP.Interface,
			// which is the VM interface name (e.g., ens18 for virtio) and may differ from the bastion's
			// interface (e.g., enp6s18).
			iface, err := system.GetDefaultInterface(ctx)
			if err != nil {
				return utils.WrapError("failed to detect network interface", err)
			}
			p.LogInfo(fmt.Sprintf("kubevip: detected bastion interface %s", iface))

			p.LogInfo(fmt.Sprintf("kubevip: adding %s to interface %s", vip, iface))
			if err := system.AddSecondaryIP(ctx, vip, iface); err != nil {
				return utils.WrapError("failed to add VIP to interface", err)
			}

			p.LogInfo(fmt.Sprintf("kubevip: sending gratuitous ARP for %s", vip))
			if err := system.SendGratuitousARP(ctx, vip, iface); err != nil {
				// Non-fatal - IP is still bound, just ARP announcement failed
				p.LogWarn(fmt.Sprintf("kubevip: gratuitous ARP failed (non-fatal): %v", err))
			}

			p.LogInfo(fmt.Sprintf("kubevip: bastion now holds VIP %s (kube-vip will take over after bootstrap)", vip))
			return nil
		}).
		MustBuild()
}
