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
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/phase"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/templates"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

const (
	StepInstallPackages   distribution.StepID = "install-packages"
	StepInstallTools      distribution.StepID = "install-tools"
	StepEnsureWorkDir     distribution.StepID = "ensure-workdir"
	StepDownloadTools     distribution.StepID = "download-tools"
	StepGenerateConfig    distribution.StepID = "generate-config"
	StepGenerateManifests distribution.StepID = "generate-manifests"
	StepGenerateKubeVIP   distribution.StepID = "generate-kubevip-manifests"
	StepInjectManifests   distribution.StepID = "inject-manifests"
	StepCompactCluster    distribution.StepID = "compact-cluster-manifests"
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

// setupSteps returns the ordered list of setup steps for the OKD setup phase.
// Complex step bodies are extracted to named methods (installSystemPackages,
// generateKubeVIPManifests, configureDNS) to keep this list readable.
func (p *Phase) setupSteps(cfg *config.Config, opts *Options) []distribution.StepDef {
	clusterDir := phase.ClusterConfigDir(opts.WorkDir)

	return []distribution.StepDef{
		{
			ID: StepInstallPackages, Name: "Install System Packages",
			Desc: "installing required system packages", NonFatal: true,
			Exec:    func(ctx context.Context) error { return p.installSystemPackages(ctx) },
			OnError: phase.WarnOnError(p.Log, "packages: system installation had warnings"),
		},
		{
			ID: StepInstallTools, Name: "Install External Tools",
			Desc: "installing core tools and addon-required tools", NonFatal: true,
			Exec:    func(ctx context.Context) error { return p.InstallExternalTools(ctx, cfg) },
			OnError: phase.WarnOnError(p.Log, "tools: external installation had warnings"),
		},
		{
			ID: StepEnsureWorkDir, Name: "Ensure Work Directory",
			Desc: "creating work directory",
			Exec: func(_ context.Context) error { return system.EnsureDir(opts.WorkDir) },
		},
		{
			ID: StepDownloadTools, Name: "Download OKD Tools",
			Desc:       fmt.Sprintf("downloading OKD tools version %s", cfg.Distribution.Version),
			SkipWhen:   func() bool { return opts.SkipDownloads },
			SkipReason: "downloads disabled",
			Exec: func(ctx context.Context) error {
				if err := p.DownloadOKDTools(ctx, cfg.Distribution.Version, opts); err != nil {
					return fmt.Errorf("failed to download OKD tools: %w", err)
				}
				p.Log.Info("tools: sha256 checksums validated successfully")
				return nil
			},
		},
		{
			ID: StepGenerateConfig, Name: "Generate Install Config",
			Desc: "generating install-config.yaml",
			Exec: func(ctx context.Context) error {
				if err := p.GenerateInstallConfig(ctx, cfg, clusterDir); err != nil {
					return fmt.Errorf("failed to generate install-config: %w", err)
				}
				p.Log.Info(fmt.Sprintf("config: install-config.yaml generated with %d masters and %d workers",
					cfg.Topology.ControlPlane.Count, cfg.Topology.Workers.Count))
				return nil
			},
		},
		{
			ID: StepGenerateManifests, Name: "Generate Manifests",
			Desc: "generating kubernetes manifests",
			Exec: func(ctx context.Context) error {
				if err := p.GenerateManifests(ctx, clusterDir); err != nil {
					return fmt.Errorf("failed to generate manifests: %w", err)
				}
				p.Log.Info("manifests: kubernetes manifests generated")
				return nil
			},
		},
		{
			ID: StepGenerateKubeVIP, Name: "Generate Kube-VIP Manifests",
			Desc: "generating kube-vip RBAC and DaemonSet manifests for VIP management",
			Exec: func(_ context.Context) error { return p.generateKubeVIPManifests(cfg, clusterDir) },
		},
		{
			ID: StepInjectManifests, Name: "Inject Custom Manifests",
			Desc: "injecting custom manifests",
			Exec: func(ctx context.Context) error {
				count, err := p.InjectCustomManifests(ctx, opts.ProjectRoot, clusterDir)
				if err != nil {
					return fmt.Errorf("failed to inject custom manifests: %w", err)
				}
				if count > 0 {
					p.Log.Info(fmt.Sprintf("manifests: injected %d custom manifest(s) from automation/config/manifests", count))
				}
				return nil
			},
		},
		{
			ID: StepCompactCluster, Name: "Inject Compact Cluster Manifests",
			Desc:       "injecting ingress controller placement for compact cluster",
			SkipWhen:   func() bool { return cfg.Topology.Workers.Count > 0 },
			SkipReason: "cluster has workers",
			Exec: func(ctx context.Context) error {
				if err := p.InjectCompactClusterManifests(ctx, clusterDir, cfg.Topology.Workers.Count, cfg.Topology.ControlPlane.Count); err != nil {
					return fmt.Errorf("failed to inject compact cluster manifests: %w", err)
				}
				p.Log.Info("manifests: injected ingress controller master placement for compact cluster")
				return nil
			},
		},
		{
			ID: StepGenerateIgnition, Name: "Generate Ignition",
			Desc: "generating ignition files",
			Exec: func(ctx context.Context) error {
				if err := p.GenerateIgnitionConfigs(ctx, clusterDir); err != nil {
					return fmt.Errorf("failed to generate ignition configs: %w", err)
				}
				p.Log.Info("ignition: configurations generated and validated")
				return nil
			},
		},
		{
			ID: StepInstallApache, Name: "Install Apache",
			Desc: "installing and configuring apache web server", NonFatal: true,
			Exec:    func(ctx context.Context) error { return p.ConfigureApache(ctx, cfg) },
			OnError: phase.WarnOnError(p.Log, "apache: installation skipped"),
		},
		{
			ID: StepDeployIgnition, Name: "Deploy Ignition",
			Desc: "deploying ignition files to apache web server",
			Exec: func(ctx context.Context) error {
				if err := p.DeployToWebServer(ctx, cfg, clusterDir); err != nil {
					return fmt.Errorf("failed to deploy to web server: %w", err)
				}
				webURL := BuildIgnitionURL(cfg.HTTPServer.IgnitionServerIP, cfg.HTTPServer.Port)
				p.Log.Info(fmt.Sprintf("ignition: deployed to web server at %s", webURL))
				return nil
			},
		},
		{
			ID: StepVerifyWebServer, Name: "Verify Web Server",
			Desc: "verifying web server accessibility",
			Exec: func(ctx context.Context) error {
				return p.VerifyWebServer(ctx, BuildIgnitionURL(cfg.HTTPServer.IgnitionServerIP, cfg.HTTPServer.Port))
			},
		},
		{
			ID: StepBuildISOs, Name: "Build ISOs",
			Desc:       "building custom CoreOS ISOs",
			SkipWhen:   func() bool { return opts.SkipISOs },
			SkipReason: "ISO building disabled",
			Exec:       func(ctx context.Context) error { return p.BuildCustomISOs(ctx, cfg, opts) },
		},
		{
			ID: StepUploadISOs, Name: "Upload ISOs",
			Desc: "uploading ISOs to Proxmox storage", NonFatal: true,
			SkipWhen:   func() bool { return opts.SkipISOs },
			SkipReason: "ISO building disabled",
			Exec: func(ctx context.Context) error {
				if err := p.UploadCustomISOsToProxmox(ctx, cfg, opts); err != nil {
					return err
				}
				p.Log.Info("iso: all custom isos uploaded to proxmox storage")
				return nil
			},
			OnError: func(err error) {
				p.Log.Warn(fmt.Sprintf("iso: upload failed: %v", err))
				p.Log.Warn("iso: you may need to upload isos manually before deploying")
			},
		},
		{
			ID: StepGenerateTfvars, Name: "Generate Terraform Variables",
			Desc: "generating terraform variables",
			Exec: func(_ context.Context) error {
				if err := p.GenerateTerraformVars(cfg, opts); err != nil {
					return fmt.Errorf("failed to generate Terraform variables: %w", err)
				}
				tfvarsPath := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", phase.GetTerraformEnv(cfg), "terraform.tfvars")
				p.Log.Info(fmt.Sprintf("terraform: configuration written to %s", tfvarsPath))
				return nil
			},
		},
		{
			ID: StepConfigureHAProxy, Name: "Configure HAProxy",
			Desc:       "configuring haproxy load balancer",
			SkipWhen:   func() bool { return opts.SkipHAProxy },
			SkipReason: "HAProxy configuration disabled",
			Exec: func(ctx context.Context) error {
				if err := p.ConfigureHAProxy(ctx, cfg, opts); err != nil {
					return fmt.Errorf("failed to configure HAProxy: %w", err)
				}
				_ = p.VerifyHAProxyPorts(ctx)
				return nil
			},
		},
		{
			ID: StepConfigureFirewall, Name: "Configure Firewall",
			Desc:       "configuring firewall rules for OKD",
			SkipWhen:   func() bool { return opts.SkipFirewall },
			SkipReason: "firewall configuration disabled",
			Exec: func(ctx context.Context) error {
				if err := firewall.ConfigureOKD(ctx, true, p.Log); err != nil {
					return err
				}
				p.Log.Info("firewall: okd rules added to firewalld")
				return nil
			},
			OnError: phase.WarnOnError(p.Log, "firewall: configuration failed"),
		},
		{
			ID: StepConfigureDNS, Name: "Configure DNS",
			Desc: "configuring dnsmasq and deploying bootstrap dns configuration", NonFatal: true,
			Exec: func(ctx context.Context) error { return p.configureDNS(ctx, cfg, opts) },
			OnError: func(err error) {
				p.Log.Warn(fmt.Sprintf("dns: configuration failed: %v", err))
				p.Log.Warn("dns: you may need to configure dns manually")
			},
		},
	}
}

// installSystemPackages installs the base OS packages required for setup.
// Extracted from the StepInstallPackages closure because the body is 20+ LOC.
func (p *Phase) installSystemPackages(ctx context.Context) error {
	sysPkgs := []string{"coreos-installer", "haproxy", p.OS.ApachePackageName(), "dnsmasq"}

	var toInstall []string
	for _, pkg := range sysPkgs {
		if !executor.CommandExists(pkg) {
			toInstall = append(toInstall, pkg)
			p.Log.Info(fmt.Sprintf("packages: %s not found", pkg))
		} else {
			p.Log.Info(fmt.Sprintf("packages: %s already installed", pkg))
		}
	}

	if len(toInstall) == 0 {
		p.Log.Info("packages: all system packages already installed")
		return nil
	}

	p.Log.Info(fmt.Sprintf("packages: installing %d missing package(s)", len(toInstall)))
	if err := packages.Install(ctx, p.Pkg, toInstall, "system dependencies", p.Log); err != nil {
		p.Log.Warn(fmt.Sprintf("packages: installation had warnings: %v", err))
	}
	return nil
}

// generateKubeVIPManifests renders and writes the kube-vip RBAC and DaemonSet
// manifests into the openshift manifests directory. Extracted from the
// StepGenerateKubeVIP closure because the body is ~40 LOC.
func (p *Phase) generateKubeVIPManifests(cfg *config.Config, clusterDir string) error {
	vip, err := netutil.ResolveVIP(cfg.Networking.Bastion.VIP, cfg.Networking.StaticIP.Start)
	if err != nil {
		return fmt.Errorf("failed to resolve VIP: %w", err)
	}

	iface := cfg.Networking.StaticIP.Interface
	if iface == "" {
		iface = "ens18" // default virtio interface on Proxmox VMs
	}

	openshiftDir := filepath.Join(clusterDir, "openshift")
	if err := system.EnsureDir(openshiftDir); err != nil {
		return fmt.Errorf("failed to ensure openshift manifests directory: %w", err)
	}

	rbacManifests, err := templates.RenderKubeVIPRBACManifests()
	if err != nil {
		return fmt.Errorf("failed to render kube-vip RBAC manifests: %w", err)
	}
	for _, m := range rbacManifests {
		path := filepath.Join(openshiftDir, m.Filename)
		if err := system.AtomicWriteString(path, m.Content, 0o644); err != nil {
			return fmt.Errorf("failed to write %s: %w", m.Filename, err)
		}
	}

	ds, err := templates.RenderKubeVIPDaemonSet(templates.KubeVIPData{
		VIPAddress: vip,
		Interface:  iface,
	})
	if err != nil {
		return fmt.Errorf("failed to render kube-vip DaemonSet manifest: %w", err)
	}
	dsPath := filepath.Join(openshiftDir, "99-kube-vip-daemonset.yaml")
	if err := system.AtomicWriteString(dsPath, ds, 0o644); err != nil {
		return fmt.Errorf("failed to write kube-vip DaemonSet manifest: %w", err)
	}

	p.Log.Info(fmt.Sprintf("kubevip: manifests generated (vip=%s, interface=%s, image=ghcr.io/kube-vip/kube-vip:%s)",
		vip, iface, templates.DefaultKubeVIPImageTag))
	return nil
}

// configureDNS wires up dnsmasq, deploys the bootstrap DNS config, and saves
// a reference copy to the work dir. Extracted from StepConfigureDNS closure.
func (p *Phase) configureDNS(ctx context.Context, cfg *config.Config, opts *Options) error {
	p.Log.Info("dns: configuring dnsmasq service")
	if err := dns.Setup(ctx, cfg.Networking.DNS, p.Log); err != nil {
		return fmt.Errorf("failed to setup dnsmasq: %w", err)
	}

	p.Log.Info("dns: deploying bootstrap dns configuration")
	if err := dns.DeployBootstrap(ctx, cfg); err != nil {
		return fmt.Errorf("failed to deploy bootstrap dns: %w", err)
	}

	// Save a copy to the work directory for reference (non-fatal).
	outputDir := filepath.Join(opts.WorkDir, "dns")
	if _, _, err := dns.GenerateBootstrapConfig(cfg, outputDir); err != nil {
		p.Log.Warn(fmt.Sprintf("dns: failed to save config copy: %v", err))
	}

	configPath, err := dns.DnsmasqConfigPath(fmt.Sprintf("okd-%s", cfg.Cluster.Name))
	if err != nil {
		return fmt.Errorf("failed to resolve dnsmasq config path: %w", err)
	}
	p.Log.Info(fmt.Sprintf("dns: dnsmasq configured at %s", configPath))
	return nil
}
