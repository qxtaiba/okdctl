package setup

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/dns"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/firewall"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/packages"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/paths"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/templates"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

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
	StepConfigureFirewall distribution.StepID = "configure-firewall"
	StepConfigureDNS      distribution.StepID = "configure-dns"
)

func systemPackages() []string {
	return []string{
		"coreos-installer",
		"haproxy",
		"httpd",
		"dnsmasq",
	}
}

func (p *Phase) newInstallPackagesStep(opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepInstallPackages, "Install System Packages").
		Description("installing required system packages").
		Fatal(false).
		Execute(func(ctx context.Context) error {
			sysPkgs := systemPackages()

			var packagesToInstall []string
			for _, pkg := range sysPkgs {
				cmdName := pkg
				if !executor.CommandExists(cmdName) {
					packagesToInstall = append(packagesToInstall, pkg)
					p.Log.Info(fmt.Sprintf("packages: %s not found", pkg))
				} else {
					p.Log.Info(fmt.Sprintf("packages: %s already installed", pkg))
				}
			}

			if len(packagesToInstall) == 0 {
				p.Log.Info("packages: all system packages already installed")
				return nil
			}

			p.Log.Info(fmt.Sprintf("packages: installing %d missing package(s) via dnf", len(packagesToInstall)))
			if err := packages.Install(ctx, packagesToInstall, "system dependencies", p.Log); err != nil {
				p.Log.Warn(fmt.Sprintf("packages: installation had warnings: %v", err))
			}

			return nil
		}).
		OnError(paths.WarnOnError(p.Log, "packages: system installation had warnings")).
		MustBuild()
}

func (p *Phase) newInstallToolsStep(cfg *config.Config) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepInstallTools, "Install External Tools").
		Description("installing core tools and addon-required tools").
		Fatal(false).
		Execute(func(ctx context.Context) error {
			return p.InstallExternalTools(ctx, cfg)
		}).
		OnError(paths.WarnOnError(p.Log, "tools: external installation had warnings")).
		MustBuild()
}

func (p *Phase) newEnsureWorkDirStep(opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepEnsureWorkDir, "Ensure Work Directory").
		Description("creating work directory").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			return system.EnsureDir(opts.WorkDir)
		}).
		MustBuild()
}

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
			p.Log.Info("tools: sha256 checksums validated successfully")
			return nil
		}).
		MustBuild()
}

func (p *Phase) newGenerateInstallConfigStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	clusterDir := paths.ClusterConfigDir(opts.WorkDir)

	return distribution.NewStepBuilder(StepGenerateConfig, "Generate Install Config").
		Description("generating install-config.yaml").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			if err := p.GenerateInstallConfig(ctx, cfg, clusterDir); err != nil {
				return utils.WrapError("failed to generate install-config", err)
			}
			p.Log.Info(fmt.Sprintf("config: install-config.yaml generated with %d masters and %d workers",
				cfg.Topology.ControlPlane.Count, cfg.Topology.Workers.Count))
			return nil
		}).
		MustBuild()
}

func (p *Phase) newGenerateManifestsStep(opts Options) distribution.ProvisioningStep {
	clusterDir := paths.ClusterConfigDir(opts.WorkDir)

	return distribution.NewStepBuilder(StepGenerateManifests, "Generate Manifests").
		Description("generating kubernetes manifests").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			if err := p.GenerateManifests(ctx, clusterDir); err != nil {
				return utils.WrapError("failed to generate manifests", err)
			}
			p.Log.Info("manifests: kubernetes manifests generated")
			return nil
		}).
		MustBuild()
}

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

			p.Log.Info(fmt.Sprintf("kubevip: manifests generated (vip=%s, interface=%s, image=ghcr.io/kube-vip/kube-vip:%s)",
				vip, iface, templates.DefaultKubeVIPImageTag))
			return nil
		}).
		MustBuild()
}

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
				p.Log.Info(fmt.Sprintf("manifests: injected %d custom manifest(s) from automation/config/manifests", count))
			}
			return nil
		}).
		MustBuild()
}

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
			p.Log.Info("manifests: injected ingress controller master placement for compact cluster")
			return nil
		}).
		MustBuild()
}

func (p *Phase) newGenerateIgnitionStep(opts Options) distribution.ProvisioningStep {
	clusterDir := paths.ClusterConfigDir(opts.WorkDir)

	return distribution.NewStepBuilder(StepGenerateIgnition, "Generate Ignition").
		Description("generating ignition files").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			if err := p.GenerateIgnitionConfigs(ctx, clusterDir); err != nil {
				return utils.WrapError("failed to generate ignition configs", err)
			}
			p.Log.Info("ignition: configurations generated and validated")
			return nil
		}).
		MustBuild()
}

func (p *Phase) newInstallApacheStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepInstallApache, "Install Apache").
		Description("installing and configuring apache web server").
		Fatal(false).
		Execute(func(ctx context.Context) error {
			return p.ConfigureApache(ctx, cfg)
		}).
		OnError(paths.WarnOnError(p.Log, "apache: installation skipped")).
		MustBuild()
}

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
			p.Log.Info(fmt.Sprintf("ignition: deployed to web server at %s", webURL))
			return nil
		}).
		MustBuild()
}

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

func (p *Phase) newUploadISOsStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepUploadISOs, "Upload ISOs").
		Description("uploading ISOs to Proxmox storage").
		Fatal(false).
		SkipWhen(func() bool { return opts.SkipISOs }).
		SkipReason("ISO building disabled").
		Execute(func(ctx context.Context) error {
			if err := p.UploadCustomISOsToProxmox(ctx, cfg, opts); err != nil {
				return err
			}
			p.Log.Info("iso: all custom isos uploaded to proxmox storage")
			return nil
		}).
		OnError(func(err error) {
			p.Log.Warn(fmt.Sprintf("iso: upload failed: %v", err))
			p.Log.Warn("iso: you may need to upload isos manually before deploying")
		}).
		MustBuild()
}

func (p *Phase) newGenerateTfvarsStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepGenerateTfvars, "Generate Terraform Variables").
		Description("generating terraform variables").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			if err := p.GenerateTerraformVars(cfg, opts); err != nil {
				return utils.WrapError("failed to generate Terraform variables", err)
			}
			tfvarsPath := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", paths.GetTerraformEnv(cfg), "terraform.tfvars")
			p.Log.Info(fmt.Sprintf("terraform: configuration written to %s", tfvarsPath))
			return nil
		}).
		MustBuild()
}

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
			_ = p.VerifyHAProxyPorts(ctx)
			return nil
		}).
		MustBuild()
}

func (p *Phase) newConfigureFirewallStep(opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepConfigureFirewall, "Configure Firewall").
		Description("configuring firewall rules for OKD").
		Fatal(true).
		SkipWhen(func() bool { return opts.SkipFirewall }).
		SkipReason("firewall configuration disabled").
		Execute(func(ctx context.Context) error {
			if err := firewall.ConfigureOKD(ctx, true, p.Log); err != nil {
				return err
			}
			p.Log.Info("firewall: okd rules added to firewalld")
			return nil
		}).
		OnError(paths.WarnOnError(p.Log, "firewall: configuration failed")).
		MustBuild()
}

func (p *Phase) newConfigureDNSStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	outputDir := filepath.Join(opts.WorkDir, "dns")
	funcs := p.dnsFunctions()

	return distribution.NewStepBuilder(StepConfigureDNS, "Configure DNS").
		Description("configuring dnsmasq and deploying bootstrap dns configuration").
		Fatal(false).
		Execute(func(ctx context.Context) error {
			p.Log.Info("dns: configuring dnsmasq service")
			if funcs.setupDnsmasq != nil {
				if err := funcs.setupDnsmasq(ctx, cfg.Networking.DNS); err != nil {
					return utils.WrapError("failed to setup dnsmasq", err)
				}
			}

			p.Log.Info("dns: deploying bootstrap dns configuration")
			if funcs.deployBootstrapDNS != nil {
				if err := funcs.deployBootstrapDNS(ctx, cfg); err != nil {
					return utils.WrapError("failed to deploy bootstrap dns", err)
				}
			}

			// Save a copy to the work directory for reference (non-fatal)
			if funcs.generateBootstrapDNSConfig != nil {
				if _, _, err := funcs.generateBootstrapDNSConfig(cfg, outputDir); err != nil {
					p.Log.Warn(fmt.Sprintf("dns: failed to save config copy: %v", err))
				}
			}

			configPath := dns.DnsmasqConfigPath(fmt.Sprintf("okd-%s", cfg.Cluster.Name))
			p.Log.Info(fmt.Sprintf("dns: dnsmasq configured at %s", configPath))
			return nil
		}).
		OnError(func(err error) {
			p.Log.Warn(fmt.Sprintf("dns: configuration failed: %v", err))
			p.Log.Warn("dns: you may need to configure dns manually")
		}).
		MustBuild()
}

