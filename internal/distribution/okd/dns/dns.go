// Package dns provides dnsmasq configuration for OKD clusters.
package dns

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// BuildConfigData assembles the DNS template data (cluster domain, node IPs,
// VIPs, upstream servers) from a validated Config. The node IP range is
// checked against machineCIDR up front so we fail early rather than midway
// through per-node calculations.
func BuildConfigData(cfg *config.Config) (templates.DNSConfigData, error) {
	if cfg == nil {
		return templates.DNSConfigData{}, fmt.Errorf("config cannot be nil")
	}

	clusterDomain := fmt.Sprintf("%s.%s", cfg.Cluster.Name, cfg.Cluster.Domain)
	staticIPStart := cfg.Networking.StaticIP.Start

	if cfg.Cluster.Name == "" {
		return templates.DNSConfigData{}, fmt.Errorf("cluster name is required")
	}
	if !config.IsValidDNSLabel(cfg.Cluster.Name) {
		return templates.DNSConfigData{}, fmt.Errorf("cluster name %q is not a valid DNS label", cfg.Cluster.Name)
	}
	if cfg.Networking.Bastion.IP == "" {
		return templates.DNSConfigData{}, fmt.Errorf("bastion IP is required")
	}
	if staticIPStart == "" {
		return templates.DNSConfigData{}, fmt.Errorf("static IP start is required")
	}

	// Validate the node IP range up front so we fail with a clear error here
	// rather than midway through per-node CalculateVMIP calls. This mirrors
	// the check in setup/nodes.go so the two paths stay in lockstep.
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
		return templates.DNSConfigData{}, fmt.Errorf("failed to calculate bootstrap IP: %w", err)
	}
	data.BootstrapNode = templates.DNSNode{
		Name: fmt.Sprintf("%s-bootstrap", cfg.Cluster.Name),
		IP:   bootstrapIP,
	}

	for i := range cfg.Topology.ControlPlane.Count {
		ip, err := netutil.CalculateVMIP(staticIPStart, i+1)
		if err != nil {
			return templates.DNSConfigData{}, fmt.Errorf("failed to calculate master%d IP: %w", i, err)
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
			return templates.DNSConfigData{}, fmt.Errorf("failed to calculate worker%d IP: %w", i, err)
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
	data, err := BuildConfigData(cfg)
	if err != nil {
		return "", "", fmt.Errorf("failed to build dns config data: %w", err)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", "", fmt.Errorf("failed to create dns config directory: %w", err)
	}

	content, err = templates.RenderDNSBootstrapConfig(&data)
	if err != nil {
		return "", "", fmt.Errorf("failed to render bootstrap dns config: %w", err)
	}

	path = filepath.Join(outputDir, "dnsmasq-bootstrap.conf")
	if err := system.AtomicWriteString(path, content, 0o644); err != nil {
		return "", "", fmt.Errorf("failed to write bootstrap dns config: %w", err)
	}

	return path, content, nil
}

// ConfigName returns the dnsmasq drop-in name for clusterName ("okd-<name>").
func ConfigName(clusterName string) string {
	return fmt.Sprintf("okd-%s", clusterName)
}

// Setup enables dnsmasq and points the system resolver at it, with
// fallbackDNS used when the cluster resolver is unavailable.
func Setup(ctx context.Context, fallbackDNS []string, logger *slog.Logger) error {
	if err := EnableDnsmasq(ctx); err != nil {
		return fmt.Errorf("failed to enable dnsmasq: %w", err)
	}
	if err := ConfigureSystemResolver(ctx, fallbackDNS, logger); err != nil {
		return fmt.Errorf("failed to configure system resolver: %w", err)
	}
	return nil
}

// DeployBootstrap writes the bootstrap-phase dnsmasq config and restarts the
// service. On validation or restart failure the previous config is restored.
func DeployBootstrap(ctx context.Context, cfg *config.Config) error {
	data, err := BuildConfigData(cfg)
	if err != nil {
		return fmt.Errorf("failed to build dns config data: %w", err)
	}

	content, err := templates.RenderDNSBootstrapConfig(&data)
	if err != nil {
		return fmt.Errorf("failed to render bootstrap dns config: %w", err)
	}

	configName := ConfigName(cfg.Cluster.Name)
	if err := WriteDnsmasqConfig(ctx, configName, content); err != nil {
		return fmt.Errorf("failed to write dnsmasq config: %w", err)
	}

	if err := validateAndRestartDnsmasq(ctx, configName); err != nil {
		return err
	}

	return nil
}

// DeployProduction writes the post-install dnsmasq config with cluster
// apps/VIP records and any custom-domain overrides, then restarts dnsmasq.
// On failure the previous config is restored.
func DeployProduction(ctx context.Context, cfg *config.Config, appsIP, kubeVipIP string, customDomains []templates.DNSCustomDomain) error {
	data, err := BuildConfigData(cfg)
	if err != nil {
		return fmt.Errorf("failed to build dns config data: %w", err)
	}
	if appsIP != "" && !config.IsValidIP(appsIP) {
		return fmt.Errorf("invalid apps IP address: %s", appsIP)
	}
	if kubeVipIP != "" && !config.IsValidIP(kubeVipIP) {
		return fmt.Errorf("invalid kube-vip IP address: %s", kubeVipIP)
	}
	for _, cd := range customDomains {
		if !config.IsValidIP(cd.IP) {
			return fmt.Errorf("invalid custom domain IP address for %s: %s", cd.Domain, cd.IP)
		}
	}
	data.AppsIP = appsIP
	if kubeVipIP != "" {
		data.KubeVipIP = kubeVipIP
	}
	data.CustomDomains = customDomains

	content, err := templates.RenderDNSProductionConfig(&data)
	if err != nil {
		return fmt.Errorf("failed to render production dns config: %w", err)
	}

	configName := ConfigName(cfg.Cluster.Name)
	if err := WriteDnsmasqConfig(ctx, configName, content); err != nil {
		return fmt.Errorf("failed to write dnsmasq config: %w", err)
	}

	if err := validateAndRestartDnsmasq(ctx, configName); err != nil {
		return err
	}

	return nil
}

// validateAndRestartDnsmasq validates the dnsmasq config, then restarts the service.
// On validation or restart failure, it restores the previous config from the .backup file.
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
		_ = system.CopyFile(backupPath, configPath)
		_ = os.Chmod(configPath, 0o644)
	}

	if err := ValidateDnsmasqConfig(ctx); err != nil {
		restore()
		return errors.Join(
			fmt.Errorf("dnsmasq config validation failed — previous config restored"),
			err,
		)
	}

	if err := RestartDnsmasq(ctx); err != nil {
		restore()
		return fmt.Errorf("failed to restart dnsmasq — previous config restored: %w", err)
	}

	// Successful restart — clean up backup.
	if system.FileExists(backupPath) {
		_ = os.RemoveAll(backupPath)
	}

	return nil
}
