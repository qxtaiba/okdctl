package setup

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/dns"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/firewall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/packages"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/system"
)

// Step IDs for the setup phase, ordered as they execute.
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

// setupSteps returns the ordered steps for the OKD setup phase, grouped
// into base / manifest / web / infra sub-methods.
func (p *Phase) setupSteps(cfg *config.Config, opts *Options) []distribution.StepDef {
	clusterDir := phase.ClusterConfigDir(opts.WorkDir)
	var steps []distribution.StepDef
	steps = append(steps, p.setupBaseSteps(cfg, opts)...)
	steps = append(steps, p.setupManifestSteps(cfg, opts, clusterDir)...)
	steps = append(steps, p.setupWebSteps(cfg, opts, clusterDir)...)
	steps = append(steps, p.setupInfraSteps(cfg, opts)...)
	return steps
}

// setupBaseSteps covers the host-level prerequisites: OS packages, external
// tools, the working directory, and the OKD installer download.
func (p *Phase) setupBaseSteps(cfg *config.Config, opts *Options) []distribution.StepDef {
	return []distribution.StepDef{
		{
			ID: StepInstallPackages, Name: "install system packages",
			Desc: "installing required system packages", NonFatal: true,
			Exec:    func(ctx context.Context) error { return p.installSystemPackages(ctx) },
			OnError: phase.WarnOnError(p.Log, "packages: system installation had warnings"),
		},
		{
			ID: StepInstallTools, Name: "install external tools",
			Desc: "installing core tools and addon-required tools", NonFatal: true,
			Exec:    func(ctx context.Context) error { return p.InstallExternalTools(ctx, cfg) },
			OnError: phase.WarnOnError(p.Log, "tools: external installation had warnings"),
		},
		{
			ID: StepEnsureWorkDir, Name: "ensure work directory",
			Desc: "creating work directory",
			Exec: func(_ context.Context) error { return system.EnsureDir(opts.WorkDir) },
		},
		{
			ID: StepDownloadTools, Name: "download okd tools",
			Desc:       fmt.Sprintf("downloading OKD tools version %s", cfg.Distribution.Version),
			SkipWhen:   func() bool { return opts.SkipDownloads },
			SkipReason: "downloads disabled",
			Exec: func(ctx context.Context) error {
				if err := p.DownloadOKDTools(ctx, cfg.Distribution.Version, opts); err != nil {
					return &errtypes.NetworkError{Msg: "failed to download OKD tools", Err: err}
				}
				p.Log.Info("tools: sha256 checksums validated successfully")
				return nil
			},
		},
	}
}

// setupManifestSteps covers install-config, k8s manifests (core, kube-vip,
// custom, compact-cluster), and ignition file generation.
func (p *Phase) setupManifestSteps(cfg *config.Config, opts *Options, clusterDir string) []distribution.StepDef {
	return []distribution.StepDef{
		{
			ID: StepGenerateConfig, Name: "generate install config",
			Desc: "generating install-config.yaml",
			Exec: func(ctx context.Context) error {
				if err := p.GenerateInstallConfig(ctx, cfg, clusterDir); err != nil {
					return &errtypes.ConfigError{Msg: "failed to generate install-config", Err: err}
				}
				p.Log.Info("config: install-config.yaml generated",
					"masters", cfg.Topology.ControlPlane.Count, "workers", cfg.Topology.Workers.Count)
				return nil
			},
		},
		{
			ID: StepGenerateManifests, Name: "generate manifests",
			Desc: "generating kubernetes manifests",
			Exec: func(ctx context.Context) error {
				if err := p.GenerateManifests(ctx, clusterDir); err != nil {
					return &errtypes.ClusterError{Msg: "failed to generate manifests", Err: err}
				}
				p.Log.Info("manifests: kubernetes manifests generated")
				return nil
			},
		},
		{
			ID: StepGenerateKubeVIP, Name: "generate kube-vip manifests",
			Desc: "generating kube-vip RBAC and DaemonSet manifests for VIP management",
			Exec: func(_ context.Context) error {
				if err := p.generateKubeVIPManifests(cfg, clusterDir); err != nil {
					return &errtypes.ConfigError{Msg: "failed to generate kube-vip manifests", Err: err}
				}
				return nil
			},
		},
		{
			ID: StepInjectManifests, Name: "inject custom manifests",
			Desc: "injecting custom manifests",
			Exec: func(ctx context.Context) error {
				count, err := p.InjectCustomManifests(ctx, opts.ProjectRoot, clusterDir)
				if err != nil {
					return &errtypes.ConfigError{Msg: "failed to inject custom manifests", Err: err}
				}
				if count > 0 {
					p.Log.Info(fmt.Sprintf("manifests: injected %d custom manifest(s) from automation/config/manifests", count))
				}
				return nil
			},
		},
		{
			ID: StepCompactCluster, Name: "inject compact cluster manifests",
			Desc:       "injecting ingress controller placement for compact cluster",
			SkipWhen:   func() bool { return cfg.Topology.Workers.Count > 0 },
			SkipReason: "cluster has workers",
			Exec: func(ctx context.Context) error {
				if err := p.InjectCompactClusterManifests(ctx, clusterDir, cfg.Topology.Workers.Count, cfg.Topology.ControlPlane.Count); err != nil {
					return &errtypes.ConfigError{Msg: "failed to inject compact cluster manifests", Err: err}
				}
				p.Log.Info("manifests: injected ingress controller master placement for compact cluster")
				return nil
			},
		},
		{
			ID: StepGenerateIgnition, Name: "generate ignition",
			Desc: "generating ignition files",
			Exec: func(ctx context.Context) error {
				if err := p.GenerateIgnitionConfigs(ctx, clusterDir); err != nil {
					return &errtypes.ClusterError{Msg: "failed to generate ignition configs", Err: err}
				}
				p.Log.Info("ignition: configurations generated and validated")
				return nil
			},
		},
	}
}

// setupWebSteps covers the apache web server for ignition delivery and the
// CoreOS ISO customization / upload pipeline.
func (p *Phase) setupWebSteps(cfg *config.Config, opts *Options, clusterDir string) []distribution.StepDef {
	return []distribution.StepDef{
		{
			ID: StepInstallApache, Name: "install apache",
			Desc: "installing and configuring apache web server", NonFatal: true,
			Exec:    func(ctx context.Context) error { return p.ConfigureApache(ctx, cfg) },
			OnError: phase.WarnOnError(p.Log, "apache: installation skipped"),
		},
		{
			ID: StepDeployIgnition, Name: "deploy ignition",
			Desc: "deploying ignition files to apache web server",
			Exec: func(ctx context.Context) error {
				if err := p.DeployToWebServer(ctx, cfg, clusterDir); err != nil {
					return &errtypes.ConfigError{Msg: "failed to deploy to web server", Err: err}
				}
				webURL := BuildIgnitionURL(cfg.HTTPServer.IgnitionServerIP, cfg.HTTPServer.Port)
				p.Log.Info(fmt.Sprintf("ignition: deployed to web server at %s", webURL))
				return nil
			},
		},
		{
			ID: StepVerifyWebServer, Name: "verify web server",
			Desc: "verifying web server accessibility",
			Exec: func(ctx context.Context) error {
				return p.VerifyWebServer(ctx, BuildIgnitionURL(cfg.HTTPServer.IgnitionServerIP, cfg.HTTPServer.Port))
			},
		},
		{
			ID: StepBuildISOs, Name: "build isos",
			Desc:       "building custom CoreOS ISOs",
			SkipWhen:   func() bool { return opts.SkipISOs },
			SkipReason: "iso building disabled",
			Exec:       func(ctx context.Context) error { return p.BuildCustomISOs(ctx, cfg, opts) },
		},
		{
			ID: StepUploadISOs, Name: "upload isos",
			Desc: "uploading ISOs to Proxmox storage", NonFatal: true,
			SkipWhen:   func() bool { return opts.SkipISOs },
			SkipReason: "iso building disabled",
			Exec: func(ctx context.Context) error {
				if err := p.UploadCustomISOsToProxmox(ctx, cfg, opts); err != nil {
					return err
				}
				p.Log.Info("iso: all custom isos uploaded to proxmox storage")
				return nil
			},
			OnError: func(err error) {
				p.Log.Warn("iso: upload failed", "err", err)
				p.Log.Warn("iso: you may need to upload isos manually before deploying")
			},
		},
	}
}

// setupInfraSteps covers the host-level infrastructure: terraform variables,
// haproxy load balancer, firewall rules, and dnsmasq.
func (p *Phase) setupInfraSteps(cfg *config.Config, opts *Options) []distribution.StepDef {
	return []distribution.StepDef{
		{
			ID: StepGenerateTfvars, Name: "generate terraform variables",
			Desc: "generating terraform variables",
			Exec: func(ctx context.Context) error {
				if err := p.GenerateTerraformVars(ctx, cfg, opts); err != nil {
					return &errtypes.ConfigError{Msg: "failed to generate Terraform variables", Err: err}
				}
				tfvarsPath := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", phase.GetTerraformEnv(cfg), "terraform.tfvars")
				p.Log.Info(fmt.Sprintf("terraform: configuration written to %s", tfvarsPath))
				return nil
			},
		},
		{
			ID: StepConfigureHAProxy, Name: "configure haproxy",
			Desc:       "configuring haproxy load balancer",
			SkipWhen:   func() bool { return opts.SkipHAProxy },
			SkipReason: "haproxy configuration disabled",
			Exec: func(ctx context.Context) error {
				if err := p.ConfigureHAProxy(ctx, cfg, opts); err != nil {
					return &errtypes.ClusterError{Msg: "failed to configure HAProxy", Err: err}
				}
				_ = p.VerifyHAProxyPorts(ctx)
				return nil
			},
		},
		{
			ID: StepConfigureFirewall, Name: "configure firewall",
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
			ID: StepConfigureDNS, Name: "configure dns",
			Desc: "configuring dnsmasq and deploying bootstrap dns configuration", NonFatal: true,
			Exec: func(ctx context.Context) error {
				if err := p.configureDNS(ctx, cfg, opts); err != nil {
					return &errtypes.ClusterError{Msg: "dns configuration failed", Err: err}
				}
				return nil
			},
			OnError: func(err error) {
				p.Log.Warn("dns: configuration failed", "err", err)
				p.Log.Warn("dns: you may need to configure dns manually")
			},
		},
	}
}

// installSystemPackages installs the base OS packages required for setup.
func (p *Phase) installSystemPackages(ctx context.Context) error {
	sysPkgs := []string{"coreos-installer", "haproxy", p.OS.ApachePackageName(), "dnsmasq"}

	var toInstall []string
	for _, pkg := range sysPkgs {
		if !executor.CommandExists(pkg) {
			toInstall = append(toInstall, pkg)
			p.Log.Debug(fmt.Sprintf("packages: %s not found", pkg))
		} else {
			p.Log.Debug(fmt.Sprintf("packages: %s already installed", pkg))
		}
	}

	if len(toInstall) == 0 {
		p.Log.Info("packages: all system packages already installed")
		return nil
	}

	p.Log.Info(fmt.Sprintf("packages: installing %d missing package(s)", len(toInstall)))
	if err := packages.Install(ctx, p.Pkg, toInstall, "system dependencies", p.Log); err != nil {
		p.Log.Warn("packages: installation had warnings", "err", err)
	}
	return nil
}

// generateKubeVIPManifests renders and writes the kube-vip RBAC and DaemonSet
// manifests into the openshift manifests directory.
func (p *Phase) generateKubeVIPManifests(cfg *config.Config, clusterDir string) error {
	vip, err := phase.ResolveClusterVIP(cfg)
	if err != nil {
		return err
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

	p.Log.Info("kubevip: manifests generated",
		"vip", vip, "interface", iface, "image", "ghcr.io/kube-vip/kube-vip:"+templates.DefaultKubeVIPImageTag)
	return nil
}

// configureDNS wires up dnsmasq, deploys the bootstrap DNS config, and saves
// a reference copy to the work dir.
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
		p.Log.Warn("dns: failed to save config copy", "err", err)
	}

	configPath, err := dns.DnsmasqConfigPath(fmt.Sprintf("okd-%s", cfg.Cluster.Name))
	if err != nil {
		return fmt.Errorf("failed to resolve dnsmasq config path: %w", err)
	}
	p.Log.Info(fmt.Sprintf("dns: dnsmasq configured at %s", configPath))
	return nil
}
