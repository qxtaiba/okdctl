package setup

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/dns"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/firewall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/platform"
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

// StepNames maps each setup StepID to its display name. StepDef literals in
// this file reference this map so each name has a single source.
var StepNames = map[distribution.StepID]string{
	StepInstallPackages:   "install system packages",
	StepInstallTools:      "install external tools",
	StepEnsureWorkDir:     "ensure work directory",
	StepDownloadTools:     "download okd tools",
	StepGenerateConfig:    "generate install config",
	StepGenerateManifests: "generate manifests",
	StepGenerateKubeVIP:   "generate kube-vip manifests",
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
			ID: StepInstallPackages, Name: StepNames[StepInstallPackages],
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "installing required system packages", NonFatal: true,
			Exec:    func(ctx context.Context) error { return p.installSystemPackages(ctx) },
			OnError: phase.WarnOnError(p.Log, "packages: system installation had warnings"),
		},
		{
			ID: StepInstallTools, Name: StepNames[StepInstallTools],
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "installing core tools and addon-required tools", NonFatal: true,
			Exec:    func(ctx context.Context) error { return p.InstallExternalTools(ctx, cfg) },
			OnError: phase.WarnOnError(p.Log, "tools: external installation had warnings"),
		},
		{
			ID: StepEnsureWorkDir, Name: StepNames[StepEnsureWorkDir],
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "creating work directory",
			Exec:      func(_ context.Context) error { return system.EnsureDir(opts.WorkDir) },
		},
		{
			ID: StepDownloadTools, Name: StepNames[StepDownloadTools],
			ReRunSafe:  distribution.ReRunSafeNo,
			Desc:       fmt.Sprintf("downloading OKD tools version %s", cfg.Distribution.Version),
			SkipWhen:   func() bool { return opts.SkipDownloads },
			SkipReason: "downloads disabled",
			AlreadyDone: func(_ context.Context) (bool, error) {
				binDir := config.BinDirOrDefault(p.BinDir)
				bins := []string{openshiftInstallBin, ocBin, kubectlBin}
				missing := slices.ContainsFunc(bins, func(b string) bool {
					return !system.FileExists(filepath.Join(binDir, b))
				})
				return !missing, nil
			},
			Exec: func(ctx context.Context) error {
				if err := p.DownloadOKDTools(ctx, cfg.Distribution.Version, opts); err != nil {
					return err
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
			ID: StepGenerateConfig, Name: StepNames[StepGenerateConfig],
			ReRunSafe: distribution.ReRunSafeNo,
			Desc:      "generating install-config.yaml",
			// install-config.yaml is consumed by openshift-install during manifest
			// generation; .backup is the stable post-state sentinel.
			AlreadyDone: func(_ context.Context) (bool, error) {
				return system.FileExists(filepath.Join(clusterDir, "install-config.yaml.backup")), nil
			},
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
			ID: StepGenerateManifests, Name: StepNames[StepGenerateManifests],
			ReRunSafe: distribution.ReRunSafeNo,
			Desc:      "generating kubernetes manifests",
			// manifests/ directory alone is unsafe: openshift-install can exit
			// non-zero mid-write, leaving a partial directory the next run sees
			// as "already done". Require both directory + .complete sentinel.
			AlreadyDone: func(_ context.Context) (bool, error) {
				return system.DirExists(filepath.Join(clusterDir, "manifests")) &&
					system.FileExists(ManifestsSentinel(clusterDir)), nil
			},
			Exec: func(ctx context.Context) error {
				if err := p.GenerateManifests(ctx, clusterDir); err != nil {
					return &errtypes.ClusterError{Msg: "failed to generate manifests", Err: err}
				}
				p.Log.Info("manifests: kubernetes manifests generated")
				return nil
			},
		},
		{
			ID: StepGenerateKubeVIP, Name: StepNames[StepGenerateKubeVIP],
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "generating kube-vip RBAC and DaemonSet manifests for VIP management",
			Exec: func(_ context.Context) error {
				if err := p.generateKubeVIPManifests(cfg, clusterDir); err != nil {
					return &errtypes.ConfigError{Msg: "failed to generate kube-vip manifests", Err: err}
				}
				return nil
			},
		},
		{
			ID: StepInjectManifests, Name: StepNames[StepInjectManifests],
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "injecting custom manifests",
			Exec: func(ctx context.Context) error {
				count, err := p.InjectCustomManifests(ctx, opts.ProjectRoot, clusterDir)
				if err != nil {
					return &errtypes.ConfigError{Msg: "failed to inject custom manifests", Err: err}
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
			ID: StepGenerateIgnition, Name: StepNames[StepGenerateIgnition],
			ReRunSafe: distribution.ReRunSafeNo,
			Desc:      "generating ignition files",
			AlreadyDone: func(_ context.Context) (bool, error) {
				return system.FileExists(IgnitionSentinel(clusterDir)), nil
			},
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
			ID: StepInstallApache, Name: StepNames[StepInstallApache],
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "installing and configuring apache web server", NonFatal: true,
			Exec:    func(ctx context.Context) error { return p.ConfigureApache(ctx, cfg, opts.ProjectRoot) },
			OnError: phase.WarnOnError(p.Log, "apache: installation skipped"),
		},
		{
			ID: StepDeployIgnition, Name: StepNames[StepDeployIgnition],
			ReRunSafe: distribution.ReRunSafeNo,
			Desc:      "deploying ignition files to apache web server",
			AlreadyDone: func(_ context.Context) (bool, error) {
				webRoot := cfg.HTTPServer.Root
				if webRoot == "" {
					webRoot = phase.DefaultHTTPServerRoot
				}
				// All files must exist: a partial-write resumes the full deploy.
				for _, name := range ignitionFilenames {
					if !system.FileExists(filepath.Join(webRoot, "ignition", name)) {
						return false, nil
					}
				}
				return true, nil
			},
			Exec: func(ctx context.Context) error {
				if err := p.DeployToWebServer(ctx, cfg, clusterDir); err != nil {
					return &errtypes.ConfigError{Msg: "failed to deploy to web server", Err: err}
				}
				webURL := BuildIgnitionURL(cfg.HTTPServer.IgnitionServerIP, cfg.HTTPServer.Port)
				p.Log.Info("ignition: deployed to web server", "url", webURL)
				return nil
			},
		},
		{
			ID: StepVerifyWebServer, Name: StepNames[StepVerifyWebServer],
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "verifying https web server accessibility",
			Exec: func(ctx context.Context) error {
				certPEM, _, err := EnsureIgnitionCert(opts.ProjectRoot, cfg.HTTPServer.IgnitionServerIP)
				if err != nil {
					return &errtypes.ConfigError{Msg: "failed to load ignition cert for verification", Err: err}
				}
				return p.VerifyWebServer(ctx, BuildIgnitionURL(cfg.HTTPServer.IgnitionServerIP, cfg.HTTPServer.Port), certPEM)
			},
		},
		{
			ID: StepBuildISOs, Name: StepNames[StepBuildISOs],
			// BuildCustomISOs fingerprint-checks per node (iso.go) and skips
			// unchanged ISOs, making repeated invocations safe.
			ReRunSafe:  distribution.ReRunSafeYes,
			Desc:       "building custom CoreOS ISOs",
			SkipWhen:   func() bool { return opts.SkipISOs },
			SkipReason: "iso building disabled",
			Exec:       func(ctx context.Context) error { return p.BuildCustomISOs(ctx, cfg, opts) },
		},
		{
			ID: StepUploadISOs, Name: StepNames[StepUploadISOs],
			ReRunSafe: distribution.ReRunSafeNo,
			Desc:      "uploading ISOs to Proxmox storage", NonFatal: true,
			SkipWhen:   func() bool { return opts.SkipISOs },
			SkipReason: "iso building disabled",
			AlreadyDone: func(ctx context.Context) (bool, error) {
				return p.isoUploadAlreadyDone(ctx, cfg, opts)
			},
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
			ID: StepGenerateTfvars, Name: StepNames[StepGenerateTfvars],
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "generating terraform variables",
			Exec: func(ctx context.Context) error {
				if err := p.GenerateTerraformVars(ctx, cfg, opts); err != nil {
					return &errtypes.ConfigError{Msg: "failed to generate Terraform variables", Err: err}
				}
				tfvarsPath := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", phase.GetTerraformEnv(cfg), "terraform.tfvars")
				p.Log.Info("terraform: configuration written", "path", tfvarsPath)
				return nil
			},
		},
		{
			ID: StepConfigureHAProxy, Name: StepNames[StepConfigureHAProxy],
			ReRunSafe:  distribution.ReRunSafeYes,
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
			ID: StepConfigureFirewall, Name: StepNames[StepConfigureFirewall],
			ReRunSafe:  distribution.ReRunSafeYes,
			Desc:       "configuring firewall rules for OKD",
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
			Desc:      "configuring dnsmasq and deploying bootstrap dns configuration", NonFatal: true,
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

func (p *Phase) installSystemPackages(ctx context.Context) error {
	sysPkgs := []string{"coreos-installer", "haproxy", p.OS.ApachePackageName(), "dnsmasq"}
	// mod_ssl is a separate rpm on RHEL/Fedora and is required for the HTTPS
	// ignition vhost's SSLEngine directive; on Debian mod_ssl ships with the
	// apache2 package and is enabled via `a2enmod ssl` instead.
	if p.OS.Family == platform.FamilyRHEL {
		sysPkgs = append(sysPkgs, "mod_ssl")
	}

	var toInstall []string
	for _, pkg := range sysPkgs {
		// mod_ssl is an apache module package, not a CLI tool — CommandExists
		// can't detect it, so we always include it in toInstall on RHEL. The
		// Pkg.Install path is idempotent so already-installed is a no-op.
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

	// Save a copy to the work directory for reference (non-fatal).
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
