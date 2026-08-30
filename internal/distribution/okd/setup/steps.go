package setup

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/dns"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/firewall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/provision"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/platform"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
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
	StepGenerateChrony    distribution.StepID = "generate-chrony-manifests"
	StepGenerateFstrim    distribution.StepID = "generate-fstrim-manifests"
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

// StepNames maps each setup StepID to its display name, the single source
// StepDef literals in this file reference.
var StepNames = map[distribution.StepID]string{
	StepInstallPackages:   "install system packages",
	StepInstallTools:      "install external tools",
	StepEnsureWorkDir:     "ensure work directory",
	StepDownloadTools:     "download okd tools",
	StepGenerateConfig:    "generate install config",
	StepGenerateManifests: "generate manifests",
	StepGenerateKubeVIP:   "generate kube-vip manifests",
	StepGenerateChrony:    "generate chrony machineconfigs",
	StepGenerateFstrim:    "generate fstrim machineconfigs",
	StepInjectManifests:   "inject custom manifests",
	StepCompactCluster:    "inject compact cluster manifests",
	StepGenerateIgnition:  "generate ignition",
	StepInstallApache:     "install apache",
	StepDeployIgnition:    "deploy ignition",
	StepVerifyWebServer:   "verify web server",
	StepBuildISOs:         "build isos",
	StepUploadISOs:        "upload isos",
	StepGenerateTfvars:    "generate terraform variables",
	StepConfigureHAProxy:  "configure haproxy",
	StepConfigureFirewall: "configure firewall",
	StepConfigureDNS:      "configure dns",
}

func (p *Phase) setupSteps(cfg *config.Config, opts *Options) []distribution.StepDef {
	clusterDir := workspace.ClusterConfigDir(opts.WorkDir)
	var steps []distribution.StepDef
	steps = append(steps, p.setupBaseSteps(cfg, opts)...)
	steps = append(steps, p.setupManifestSteps(cfg, opts, clusterDir)...)
	steps = append(steps, p.setupWebSteps(cfg, opts, clusterDir)...)
	steps = append(steps, p.setupInfraSteps(cfg, opts)...)
	return steps
}

func (p *Phase) setupBaseSteps(cfg *config.Config, opts *Options) []distribution.StepDef {
	return []distribution.StepDef{
		{
			ID: StepInstallPackages, Name: StepNames[StepInstallPackages],
			ReRunSafe: distribution.ReRunSafeYes,
			NonFatal:  true,
			Exec:      func(ctx context.Context) error { return p.installSystemPackages(ctx) },
			OnError:   phase.WarnOnError(p.Log, "packages: system installation had warnings"),
		},
		{
			ID: StepInstallTools, Name: StepNames[StepInstallTools],
			ReRunSafe: distribution.ReRunSafeYes,
			NonFatal:  true,
			Exec:      func(ctx context.Context) error { return p.InstallExternalTools(ctx, cfg) },
			OnError:   phase.WarnOnError(p.Log, "tools: external installation had warnings"),
		},
		{
			ID: StepEnsureWorkDir, Name: StepNames[StepEnsureWorkDir],
			ReRunSafe: distribution.ReRunSafeYes,
			Exec:      func(_ context.Context) error { return system.EnsureDir(opts.WorkDir) },
		},
		{
			ID: StepDownloadTools, Name: StepNames[StepDownloadTools],
			ReRunSafe:  distribution.ReRunSafeNo,
			SkipWhen:   func() bool { return opts.SkipDownloads },
			SkipReason: "downloads disabled",
			// Presence alone is version-blind; DownloadOKDTools' sentinel
			// carries the version so resume detects a change.
			AlreadyDone: func(_ context.Context) (bool, error) {
				return downloadToolsAlreadyDone(config.BinDirOrDefault(p.BinDir), cfg.Distribution.Version), nil
			},
			Exec: func(ctx context.Context) error {
				if err := p.DownloadOKDTools(ctx, cfg.Distribution.Version, opts); err != nil {
					return err
				}
				p.Log.Info("tools: sha256 checksums validated")
				return nil
			},
		},
	}
}

func (p *Phase) setupManifestSteps(cfg *config.Config, opts *Options, clusterDir string) []distribution.StepDef {
	return []distribution.StepDef{
		{
			ID: StepGenerateConfig, Name: StepNames[StepGenerateConfig],
			ReRunSafe: distribution.ReRunSafeNo,
			// install-config.yaml is consumed during manifest generation;
			// .backup is the stable post-state sentinel.
			AlreadyDone: func(_ context.Context) (bool, error) {
				return system.FileExists(filepath.Join(clusterDir, "install-config.yaml.backup")), nil
			},
			Exec: func(ctx context.Context) error {
				if err := p.generateInstallConfig(ctx, cfg, clusterDir); err != nil {
					return &errtypes.ConfigError{Msg: "generate install-config", Err: err}
				}
				p.Log.Info("config: install-config.yaml generated",
					"masters", cfg.Topology.ControlPlane.Count, "workers", cfg.Topology.Workers.Count)
				return nil
			},
		},
		{
			ID: StepGenerateManifests, Name: StepNames[StepGenerateManifests],
			ReRunSafe: distribution.ReRunSafeNo,
			// manifests/ alone is unsafe — a partial mid-write dir would look
			// done; require .complete or the ignition sentinel too.
			AlreadyDone: func(_ context.Context) (bool, error) {
				return manifestsGenerated(clusterDir), nil
			},
			Exec: func(ctx context.Context) error {
				if err := p.GenerateManifests(ctx, clusterDir); err != nil {
					return &errtypes.ClusterError{Msg: "generate manifests", Err: err}
				}
				p.Log.Info("manifests: kubernetes manifests generated")
				return nil
			},
		},
		{
			ID: StepGenerateKubeVIP, Name: StepNames[StepGenerateKubeVIP],
			ReRunSafe: distribution.ReRunSafeYes,
			Exec: func(_ context.Context) error {
				if err := p.generateKubeVIPManifests(cfg, clusterDir); err != nil {
					return &errtypes.ConfigError{Msg: "generate kube-vip manifests", Err: err}
				}
				return nil
			},
		},
		{
			ID: StepGenerateChrony, Name: StepNames[StepGenerateChrony],
			ReRunSafe: distribution.ReRunSafeYes,
			Exec: func(_ context.Context) error {
				if err := p.generateChronyManifests(cfg, clusterDir); err != nil {
					return &errtypes.ConfigError{Msg: "generate chrony machineconfigs", Err: err}
				}
				return nil
			},
		},
		{
			ID: StepGenerateFstrim, Name: StepNames[StepGenerateFstrim],
			ReRunSafe: distribution.ReRunSafeYes,
			Exec: func(_ context.Context) error {
				if err := p.generateFstrimManifests(clusterDir); err != nil {
					return &errtypes.ConfigError{Msg: "generate fstrim machineconfigs", Err: err}
				}
				return nil
			},
		},
		{
			ID: StepInjectManifests, Name: StepNames[StepInjectManifests],
			ReRunSafe: distribution.ReRunSafeYes,
			Exec: func(ctx context.Context) error {
				count, err := p.InjectCustomManifests(ctx, opts.ProjectRoot, clusterDir)
				if err != nil {
					return &errtypes.ConfigError{Msg: "inject custom manifests", Err: err}
				}
				if count > 0 {
					p.Log.Info("manifests: injected custom manifests", "count", count)
				}
				return nil
			},
		},
		{
			ID: StepCompactCluster, Name: StepNames[StepCompactCluster],
			ReRunSafe:  distribution.ReRunSafeYes,
			SkipWhen:   func() bool { return cfg.Topology.Workers.Count > 0 },
			SkipReason: "cluster has workers",
			Exec: func(ctx context.Context) error {
				if err := p.InjectCompactClusterManifests(ctx, clusterDir, cfg.Topology.Workers.Count, cfg.Topology.ControlPlane.Count); err != nil {
					return &errtypes.ConfigError{Msg: "inject compact cluster manifests", Err: err}
				}
				p.Log.Info("manifests: injected ingress controller master placement for compact cluster")
				return nil
			},
		},
		{
			ID: StepGenerateIgnition, Name: StepNames[StepGenerateIgnition],
			ReRunSafe: distribution.ReRunSafeNo,
			AlreadyDone: func(_ context.Context) (bool, error) {
				return system.FileExists(IgnitionSentinel(clusterDir)), nil
			},
			Exec: func(ctx context.Context) error {
				if err := p.GenerateIgnitionConfigs(ctx, clusterDir); err != nil {
					return &errtypes.ClusterError{Msg: "generate ignition configs", Err: err}
				}
				p.Log.Info("ignition: configurations generated and validated")
				return nil
			},
		},
	}
}

func (p *Phase) setupWebSteps(cfg *config.Config, opts *Options, clusterDir string) []distribution.StepDef {
	return []distribution.StepDef{
		{
			ID: StepInstallApache, Name: StepNames[StepInstallApache],
			ReRunSafe: distribution.ReRunSafeYes,
			NonFatal:  true,
			Exec:      func(ctx context.Context) error { return p.ConfigureApache(ctx, cfg, opts.ProjectRoot) },
			OnError:   phase.WarnOnError(p.Log, "apache: installation skipped"),
		},
		{
			ID: StepDeployIgnition, Name: StepNames[StepDeployIgnition],
			ReRunSafe: distribution.ReRunSafeNo,
			// Content identity, not existence: crash-resume regenerates
			// ignition with a fresh CA, so a stale webroot copy would wedge the
			// install.
			AlreadyDone: func(_ context.Context) (bool, error) {
				webRoot := cfg.HTTPServer.Root
				if webRoot == "" {
					webRoot = phase.DefaultHTTPServerRoot
				}
				return provision.IgnitionDeployAlreadyDone(clusterDir, webRoot), nil
			},
			Exec: func(ctx context.Context) error {
				if err := p.DeployToWebServer(ctx, cfg, clusterDir); err != nil {
					return &errtypes.ConfigError{Msg: "deploy to web server", Err: err}
				}
				webURL := provision.BuildIgnitionURL(cfg.HTTPServer.IgnitionServerIP)
				p.Log.Info("ignition: deployed to web server", "url", webURL)
				return nil
			},
		},
		{
			ID: StepVerifyWebServer, Name: StepNames[StepVerifyWebServer],
			ReRunSafe: distribution.ReRunSafeYes,
			Exec: func(ctx context.Context) error {
				certPEM, _, err := provision.EnsureIgnitionCert(opts.ProjectRoot, cfg.HTTPServer.IgnitionServerIP)
				if err != nil {
					return &errtypes.ConfigError{Msg: "load ignition cert for verification", Err: err}
				}
				return p.VerifyWebServer(ctx, provision.BuildIgnitionURL(cfg.HTTPServer.IgnitionServerIP), certPEM)
			},
		},
		{
			ID: StepBuildISOs, Name: StepNames[StepBuildISOs],
			// BuildCustomISOs fingerprint-checks per node (iso.go), skipping unchanged ISOs on repeat runs.
			ReRunSafe:  distribution.ReRunSafeYes,
			SkipWhen:   func() bool { return opts.SkipISOs },
			SkipReason: "iso building disabled",
			Exec:       func(ctx context.Context) error { return p.BuildCustomISOs(ctx, cfg, opts.provisionOpts()) },
		},
		{
			ID: StepUploadISOs, Name: StepNames[StepUploadISOs],
			ReRunSafe:  distribution.ReRunSafeNo,
			NonFatal:   true,
			SkipWhen:   func() bool { return opts.SkipISOs },
			SkipReason: "iso building disabled",
			AlreadyDone: func(ctx context.Context) (bool, error) {
				return p.ISOUploadAlreadyDone(ctx, cfg, opts.provisionOpts())
			},
			Exec: func(ctx context.Context) error {
				if err := p.UploadCustomISOsToProxmox(ctx, cfg, opts.provisionOpts()); err != nil {
					return err
				}
				p.Log.Info("iso: all custom isos uploaded to proxmox storage")
				return nil
			},
			OnError: func(err error) {
				p.Log.Warn("iso: upload failed", "err", err, "hint", "upload isos manually before deploying")
			},
		},
	}
}

func (p *Phase) setupInfraSteps(cfg *config.Config, opts *Options) []distribution.StepDef {
	return []distribution.StepDef{
		{
			ID: StepGenerateTfvars, Name: StepNames[StepGenerateTfvars],
			ReRunSafe: distribution.ReRunSafeYes,
			Exec: func(ctx context.Context) error {
				if err := p.GenerateTerraformVars(ctx, cfg, opts); err != nil {
					return &errtypes.ConfigError{Msg: "generate Terraform variables", Err: err}
				}
				tfvarsPath := filepath.Join(workspace.TerraformEnvDir(opts.ProjectRoot, cfg.TerraformEnvName()), "terraform.tfvars")
				p.Log.Info("terraform: configuration written", "path", tfvarsPath)
				return nil
			},
		},
		{
			ID: StepConfigureHAProxy, Name: StepNames[StepConfigureHAProxy],
			ReRunSafe:  distribution.ReRunSafeYes,
			SkipWhen:   func() bool { return opts.SkipHAProxy },
			SkipReason: "haproxy configuration disabled",
			Exec: func(ctx context.Context) error {
				if err := p.ConfigureHAProxy(ctx, cfg, opts); err != nil {
					return &errtypes.ClusterError{Msg: "configure HAProxy", Err: err}
				}
				_ = p.VerifyHAProxyPorts(ctx)
				return nil
			},
		},
		{
			ID: StepConfigureFirewall, Name: StepNames[StepConfigureFirewall],
			ReRunSafe:  distribution.ReRunSafeYes,
			NonFatal:   true,
			SkipWhen:   func() bool { return opts.SkipFirewall },
			SkipReason: "firewall configuration disabled",
			Exec: func(ctx context.Context) error {
				if err := firewall.New(firewall.WithLogger(p.Log)).ConfigureOKD(ctx, true); err != nil {
					return err
				}
				p.Log.Info("firewall: okd rules added to firewalld")
				return nil
			},
			OnError: phase.WarnOnError(p.Log, "firewall: configuration failed"),
		},
		{
			ID: StepConfigureDNS, Name: StepNames[StepConfigureDNS],
			ReRunSafe: distribution.ReRunSafeYes,
			NonFatal:  true,
			Exec: func(ctx context.Context) error {
				if err := p.configureDNS(ctx, cfg, opts); err != nil {
					return &errtypes.ClusterError{Msg: "dns configuration failed", Err: err}
				}
				return nil
			},
			OnError: func(err error) {
				p.Log.Warn("dns: configuration failed", "err", err, "hint", "configure dns manually")
			},
		},
	}
}

func (p *Phase) installSystemPackages(ctx context.Context) error {
	sysPkgs := []string{"coreos-installer", "haproxy", p.OS.ApachePackageName(), "dnsmasq"}
	// mod_ssl is a separate RHEL/Fedora rpm for the ignition vhost's SSLEngine;
	// Debian ships it with apache2 (a2enmod ssl).
	if p.OS.Family == platform.FamilyRHEL {
		sysPkgs = append(sysPkgs, "mod_ssl")
	}

	var toInstall []string
	for _, pkg := range sysPkgs {
		// mod_ssl is an apache module, not a CLI tool, so CommandExists can't
		// see it; always queue it and let idempotent Pkg.Install no-op.
		if pkg == "mod_ssl" {
			toInstall = append(toInstall, pkg)
			continue
		}
		if !executor.CommandExists(pkg) {
			toInstall = append(toInstall, pkg)
			p.Log.Debug("packages: not found", "pkg", pkg)
		} else {
			p.Log.Debug("packages: already installed", "pkg", pkg)
		}
	}

	if len(toInstall) == 0 {
		p.Log.Info("packages: all system packages already installed")
		return nil
	}

	p.Log.Info("packages: installing missing", "count", len(toInstall))
	if err := p.Pkg.Install(ctx, toInstall); err != nil {
		p.Log.Warn("packages: installation had warnings", "err", err)
	}
	return nil
}

func (p *Phase) generateKubeVIPManifests(cfg *config.Config, clusterDir string) error {
	vip, err := phase.ResolveClusterVIP(cfg)
	if err != nil {
		return err
	}

	iface := cfg.Networking.StaticIP.Interface
	if iface == "" {
		iface = netutil.DefaultProxmoxIface
	}

	openshiftDir := filepath.Join(clusterDir, openshiftSubdir)
	if err := system.EnsureDir(openshiftDir); err != nil {
		return fmt.Errorf("ensure openshift manifests directory: %w", err)
	}

	rbacManifests, err := templates.RenderKubeVIPRBACManifests()
	if err != nil {
		return fmt.Errorf("render kube-vip RBAC manifests: %w", err)
	}
	for _, m := range rbacManifests {
		path := filepath.Join(openshiftDir, m.Filename)
		if err := system.AtomicWriteString(path, m.Content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", m.Filename, err)
		}
	}

	ds, err := templates.RenderKubeVIPDaemonSet(templates.KubeVIPData{
		VIPAddress: vip,
		Interface:  iface,
	})
	if err != nil {
		return fmt.Errorf("render kube-vip DaemonSet manifest: %w", err)
	}
	dsPath := filepath.Join(openshiftDir, "99-kube-vip-daemonset.yaml")
	if err := system.AtomicWriteString(dsPath, ds, 0o644); err != nil {
		return fmt.Errorf("write kube-vip DaemonSet manifest: %w", err)
	}

	p.Log.Info("kubevip: manifests generated",
		"vip", vip, "interface", iface, "image", "ghcr.io/kube-vip/kube-vip:"+templates.DefaultKubeVIPImageTag)
	return nil
}

func (p *Phase) configureDNS(ctx context.Context, cfg *config.Config, opts *Options) error {
	p.Log.Info("dns: configuring dnsmasq service")
	if err := dns.Setup(ctx, cfg.Networking.DNS, p.Log); err != nil {
		return fmt.Errorf("setup dnsmasq: %w", err)
	}

	p.Log.Info("dns: deploying bootstrap dns configuration")
	if err := dns.DeployBootstrap(ctx, cfg); err != nil {
		return fmt.Errorf("deploy bootstrap dns: %w", err)
	}

	outputDir := filepath.Join(opts.WorkDir, "dns")
	if _, _, err := dns.GenerateBootstrapConfig(cfg, outputDir); err != nil {
		p.Log.Warn("dns: failed to save config copy", "err", err)
	}

	configPath, err := dns.DnsmasqConfigPath(fmt.Sprintf("okd-%s", cfg.Cluster.Name))
	if err != nil {
		return fmt.Errorf("resolve dnsmasq config path: %w", err)
	}
	p.Log.Info("dns: dnsmasq configured", "path", configPath)
	return nil
}
