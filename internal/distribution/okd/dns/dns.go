// Package dns provides dnsmasq configuration for OKD clusters. Exported
// service ops (EnableDnsmasq, RestartDnsmasq, ValidateDnsmasqConfig,
// ConfigureSystemResolver, IsNetworkManagerActive) are the package's public
// API surface even though today's only callers are intra-package.
package dns

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// buildConfigData assembles the DNS template data (cluster domain, node IPs,
// VIPs, upstream servers) from a validated Config. The node IP range is
// checked against machineCIDR up front so we fail early rather than midway
// through per-node calculations.
func buildConfigData(cfg *config.Config) (templates.DNSConfigData, error) {
	if cfg == nil {
		return templates.DNSConfigData{}, &errtypes.ConfigError{Msg: "config cannot be nil"}
	}

	clusterDomain := fmt.Sprintf("%s.%s", cfg.Cluster.Name, cfg.Cluster.Domain)
	staticIPStart := cfg.Networking.StaticIP.Start

	if cfg.Cluster.Name == "" {
		return templates.DNSConfigData{}, &errtypes.ConfigError{Msg: "cluster name is required"}
	}
	if !config.IsValidDNSLabel(cfg.Cluster.Name) {
		return templates.DNSConfigData{}, &errtypes.ConfigError{Msg: fmt.Sprintf("cluster name %q is not a valid DNS label", cfg.Cluster.Name)}
	}
	if cfg.Networking.Bastion.IP == "" {
		return templates.DNSConfigData{}, &errtypes.ConfigError{Msg: "bastion IP is required"}
	}
	if staticIPStart == "" {
		return templates.DNSConfigData{}, &errtypes.ConfigError{Msg: "static IP start is required"}
	}

	// Mirrors the check in setup/nodes.go so the two paths stay in lockstep.
	totalNodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
	if err := netutil.ValidateIPRangeInCIDR(staticIPStart, totalNodes, cfg.Networking.MachineCIDR); err != nil {
		return templates.DNSConfigData{}, err
	}

	kubeVipIP, err := phase.ResolveClusterVIP(cfg)
	if err != nil {
		return templates.DNSConfigData{}, err
	}

	data := templates.DNSConfigData{
		ClusterName:   cfg.Cluster.Name,
		ClusterDomain: clusterDomain,
		BastionIP:     cfg.Networking.Bastion.IP,
		KubeVipIP:     kubeVipIP,
		UpstreamDNS:   cfg.Networking.DNS,
	}

	bootstrapIP, err := netutil.CalculateVMIP(staticIPStart, 0)
	if err != nil {
		return templates.DNSConfigData{}, fmt.Errorf("calculate bootstrap IP: %w", err)
	}
	data.BootstrapNode = templates.DNSNode{
		Name: fmt.Sprintf("%s-bootstrap", cfg.Cluster.Name),
		IP:   bootstrapIP,
	}

	for i := range cfg.Topology.ControlPlane.Count {
		ip, err := netutil.CalculateVMIP(staticIPStart, i+1)
		if err != nil {
			return templates.DNSConfigData{}, fmt.Errorf("calculate master%d IP: %w", i, err)
		}
		data.MasterNodes = append(data.MasterNodes, templates.DNSNode{
			Name: fmt.Sprintf("%s-master%d", cfg.Cluster.Name, i),
			IP:   ip,
		})
	}

	workerStartIndex := cfg.Topology.ControlPlane.Count + 1
	for i := range cfg.Topology.Workers.Count {
		ip, err := netutil.CalculateVMIP(staticIPStart, workerStartIndex+i)
		if err != nil {
			return templates.DNSConfigData{}, fmt.Errorf("calculate worker%d IP: %w", i, err)
		}
		data.WorkerNodes = append(data.WorkerNodes, templates.DNSNode{
			Name: fmt.Sprintf("%s-worker%d", cfg.Cluster.Name, i),
			IP:   ip,
		})
	}

	return data, nil
}

// GenerateBootstrapConfig renders the bootstrap-phase dnsmasq config to
// outputDir/dnsmasq-bootstrap.conf and returns its path and content.
func GenerateBootstrapConfig(cfg *config.Config, outputDir string) (path, content string, err error) {
	data, err := buildConfigData(cfg)
	if err != nil {
		return "", "", fmt.Errorf("build dns config data: %w", err)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create dns config directory: %w", err)
	}

	content, err = templates.RenderDNSBootstrapConfig(&data)
	if err != nil {
		return "", "", fmt.Errorf("render bootstrap dns config: %w", err)
	}

	path = filepath.Join(outputDir, "dnsmasq-bootstrap.conf")
	if err := system.AtomicWriteString(path, content, 0o644); err != nil {
		return "", "", fmt.Errorf("write bootstrap dns config: %w", err)
	}

	return path, content, nil
}

// configName returns the dnsmasq drop-in name for clusterName ("okd-<name>").
func configName(clusterName string) string {
	return fmt.Sprintf("okd-%s", clusterName)
}

// IsBootstrapDNS reports whether the on-disk dnsmasq config is still in
// bootstrap state (api.* resolves to the bastion IP rather than the kube-vip
// VIP). Returns false when the config file is absent so the caller can treat
// missing config as "nothing to reconcile" rather than a hard error.
//
// Match is line-exact rather than substring because IP suffixes alias (a
// production VIP of "10.0.0.10" would match the bastion "10.0.0.1" prefix).
func IsBootstrapDNS(cfg *config.Config) (bool, error) {
	cn := configName(cfg.Cluster.Name)
	path, err := DnsmasqConfigPath(cn)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read dnsmasq config: %w", err)
	}
	clusterDomain := fmt.Sprintf("%s.%s", cfg.Cluster.Name, cfg.Cluster.Domain)
	want := fmt.Sprintf("address=/api.%s/%s", clusterDomain, cfg.Networking.Bastion.IP)
	for line := range strings.Lines(string(data)) {
		if strings.TrimSpace(line) == want {
			return true, nil
		}
	}
	return false, nil
}

// Setup enables dnsmasq and points the system resolver at it, with
// fallbackDNS used when the cluster resolver is unavailable.
func Setup(ctx context.Context, fallbackDNS []string, logger *slog.Logger) error {
	if err := EnableDnsmasq(ctx); err != nil {
		return fmt.Errorf("enable dnsmasq: %w", err)
	}
	if err := ConfigureSystemResolver(ctx, fallbackDNS, logger); err != nil {
		return fmt.Errorf("configure system resolver: %w", err)
	}
	return nil
}

// DeployBootstrap writes the bootstrap-phase dnsmasq config and restarts the
// service. On validation or restart failure the previous config is restored.
func DeployBootstrap(ctx context.Context, cfg *config.Config) error {
	data, err := buildConfigData(cfg)
	if err != nil {
		return fmt.Errorf("build dns config data: %w", err)
	}

	content, err := templates.RenderDNSBootstrapConfig(&data)
	if err != nil {
		return fmt.Errorf("render bootstrap dns config: %w", err)
	}

	cn := configName(cfg.Cluster.Name)
	if err := writeDnsmasqConfig(ctx, cn, content); err != nil {
		return fmt.Errorf("write dnsmasq config: %w", err)
	}

	if err := validateAndRestartDnsmasq(ctx, cn); err != nil {
		return err
	}

	return nil
}

// DeployProduction writes the post-install dnsmasq config with cluster
// apps/VIP records and any custom-domain overrides, then restarts dnsmasq.
// On failure the previous config is restored.
func DeployProduction(ctx context.Context, cfg *config.Config, appsIP, kubeVipIP string, customDomains []templates.DNSCustomDomain) error {
	data, err := buildConfigData(cfg)
	if err != nil {
		return fmt.Errorf("build dns config data: %w", err)
	}
	if appsIP != "" && !config.IsValidIP(appsIP) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("invalid apps IP address: %s", appsIP)}
	}
	if kubeVipIP != "" && !config.IsValidIP(kubeVipIP) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("invalid kube-vip IP address: %s", kubeVipIP)}
	}
	for _, cd := range customDomains {
		if !config.IsValidIP(cd.IP) {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("invalid custom domain IP address for %s: %s", cd.Domain, cd.IP)}
		}
	}
	data.AppsIP = appsIP
	if kubeVipIP != "" {
		data.KubeVipIP = kubeVipIP
	}
	data.CustomDomains = customDomains

	content, err := templates.RenderDNSProductionConfig(&data)
	if err != nil {
		return fmt.Errorf("render production dns config: %w", err)
	}

	cn := configName(cfg.Cluster.Name)
	if err := writeDnsmasqConfig(ctx, cn, content); err != nil {
		return fmt.Errorf("write dnsmasq config: %w", err)
	}

	if err := validateAndRestartDnsmasq(ctx, cn); err != nil {
		return err
	}

	return nil
}

// recoveryRestartTimeout bounds the best-effort dnsmasq restart issued after
// a failed restart forced a config restore.
const recoveryRestartTimeout = 30 * time.Second

// validateAndRestartDnsmasq validates the dnsmasq config, then restarts the service.
// On validation or restart failure, it restores the previous config from the .backup
// file; a restart failure additionally re-restarts dnsmasq so the running service
// converges to the restored config.
func validateAndRestartDnsmasq(ctx context.Context, configName string) error {
	configPath, err := DnsmasqConfigPath(configName)
	if err != nil {
		return err
	}
	backupPath := configPath + ".backup"

	restore := func() {
		if !system.FileExists(backupPath) {
			return
		}
		// No follow-up os.Chmod: system.CopyFile preserves the source mode
		// at open time, and os.Chmod follows symlinks at the destination.
		_ = system.CopyFile(backupPath, configPath)
	}

	if err := validateDnsmasqConfigFn(ctx); err != nil {
		restore()
		return fmt.Errorf("dnsmasq config validation failed — previous config restored: %w", err)
	}

	if err := restartDnsmasqFn(ctx); err != nil {
		restore()
		// Restart once more so the running service converges to the restored
		// config instead of staying down (or up on the rejected one). Detached
		// from ctx: a Ctrl-C that killed the first restart would otherwise
		// doom the recovery restart before it starts.
		rCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recoveryRestartTimeout)
		defer cancel()
		if rErr := restartDnsmasqFn(rCtx); rErr != nil {
			return fmt.Errorf("restart dnsmasq — previous config restored on disk but dnsmasq restart with it also failed: %w", errors.Join(err, rErr))
		}
		return fmt.Errorf("restart dnsmasq — previous config restored and service restarted with it: %w", err)
	}

	// Successful restart — clean up backup.
	if system.FileExists(backupPath) {
		_ = os.RemoveAll(backupPath)
	}

	return nil
}
