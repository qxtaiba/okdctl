// Package dns provides DNS configuration utilities for OKD clusters.
// It handles dnsmasq configuration generation and deployment for both
// bootstrap and production phases of cluster installation.
package dns

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/templates"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// BuildConfigData creates DNS template data from the cluster configuration.
// Returns an error if required IPs cannot be calculated.
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

	// Derive VIP from static IP configuration (uses .10 as last octet by convention)
	kubeVipIP := netutil.DeriveVIPFromStaticIP(staticIPStart)
	if kubeVipIP == "" {
		return templates.DNSConfigData{}, fmt.Errorf("failed to derive VIP from static IP start: %s", staticIPStart)
	}

	data := templates.DNSConfigData{
		ClusterName:   cfg.Cluster.Name,
		ClusterDomain: clusterDomain,
		BastionIP:     cfg.Networking.Bastion.IP,
		KubeVipIP:     kubeVipIP, // VIP for API endpoints - used from day 1
		UpstreamDNS:   cfg.Networking.DNS, // Forward external queries to user's DNS servers
	}

	bootstrapIP, err := netutil.CalculateVMIP(staticIPStart, 0)
	if err != nil {
		return templates.DNSConfigData{}, utils.WrapError("failed to calculate bootstrap IP", err)
	}
	data.BootstrapNode = templates.DNSNode{
		Name: fmt.Sprintf("%s-bootstrap", cfg.Cluster.Name),
		IP:   bootstrapIP,
	}

	for i := 0; i < cfg.Topology.ControlPlane.Count; i++ {
		ip, err := netutil.CalculateVMIP(staticIPStart, i+1)
		if err != nil {
			return templates.DNSConfigData{}, utils.WrapErrorf(err, "failed to calculate master%d IP", i)
		}
		data.MasterNodes = append(data.MasterNodes, templates.DNSNode{
			Name: fmt.Sprintf("%s-master%d", cfg.Cluster.Name, i),
			IP:   ip,
		})
	}

	workerStartIndex := cfg.Topology.ControlPlane.Count + 1
	for i := 0; i < cfg.Topology.Workers.Count; i++ {
		ip, err := netutil.CalculateVMIP(staticIPStart, workerStartIndex+i)
		if err != nil {
			return templates.DNSConfigData{}, utils.WrapErrorf(err, "failed to calculate worker%d IP", i)
		}
		data.WorkerNodes = append(data.WorkerNodes, templates.DNSNode{
			Name: fmt.Sprintf("%s-worker%d", cfg.Cluster.Name, i),
			IP:   ip,
		})
	}

	return data, nil
}

// GenerateBootstrapConfig generates only the bootstrap DNS config.
// Returns the path to the generated file and the config content.
func GenerateBootstrapConfig(cfg *config.Config, outputDir string) (path string, content string, err error) {
	data, err := BuildConfigData(cfg)
	if err != nil {
		return "", "", utils.WrapError("failed to build dns config data", err)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", "", utils.WrapError("failed to create dns config directory", err)
	}

	content, err = templates.RenderDNSBootstrapConfig(data)
	if err != nil {
		return "", "", utils.WrapError("failed to render bootstrap dns config", err)
	}

	path = filepath.Join(outputDir, "dnsmasq-bootstrap.conf")
	if err := system.AtomicWriteString(path, content, 0644); err != nil {
		return "", "", utils.WrapError("failed to write bootstrap dns config", err)
	}

	return path, content, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// DNSMASQ DEPLOYMENT FUNCTIONS
// ═══════════════════════════════════════════════════════════════════════════════

// ConfigName returns the dnsmasq config file name for a cluster.
func ConfigName(clusterName string) string {
	return fmt.Sprintf("okd-%s", clusterName)
}

// Setup enables dnsmasq and configures the system resolver to use it.
func Setup(ctx context.Context, fallbackDNS []string) error {
	if err := system.EnableDnsmasq(ctx); err != nil {
		return utils.WrapError("failed to enable dnsmasq", err)
	}
	if err := system.ConfigureSystemResolver(ctx, fallbackDNS); err != nil {
		return utils.WrapError("failed to configure system resolver", err)
	}
	return nil
}

// DeployBootstrap generates and deploys the bootstrap DNS config to dnsmasq.
func DeployBootstrap(ctx context.Context, cfg *config.Config) error {
	data, err := BuildConfigData(cfg)
	if err != nil {
		return utils.WrapError("failed to build dns config data", err)
	}

	content, err := templates.RenderDNSBootstrapConfig(data)
	if err != nil {
		return utils.WrapError("failed to render bootstrap dns config", err)
	}

	configName := ConfigName(cfg.Cluster.Name)
	if err := system.WriteDnsmasqConfig(ctx, configName, content); err != nil {
		return utils.WrapError("failed to write dnsmasq config", err)
	}

	if err := system.RestartDnsmasq(ctx); err != nil {
		return utils.WrapError("failed to restart dnsmasq", err)
	}

	return nil
}

// DeployProduction generates and deploys the production DNS config to dnsmasq.
// kubeVipIP is the kube-vip VIP for API endpoints (optional, falls back to bastion if empty).
// appsIP is the MetalLB-assigned IP for the ingress router.
func DeployProduction(ctx context.Context, cfg *config.Config, appsIP, kubeVipIP string) error {
	data, err := BuildConfigData(cfg)
	if err != nil {
		return utils.WrapError("failed to build dns config data", err)
	}
	if appsIP != "" {
		if net.ParseIP(appsIP) == nil {
			return fmt.Errorf("invalid apps IP address: %s", appsIP)
		}
	}
	if kubeVipIP != "" {
		if net.ParseIP(kubeVipIP) == nil {
			return fmt.Errorf("invalid kube-vip IP address: %s", kubeVipIP)
		}
	}
	data.AppsIP = appsIP
	data.KubeVipIP = kubeVipIP

	content, err := templates.RenderDNSProductionConfig(data)
	if err != nil {
		return utils.WrapError("failed to render production dns config", err)
	}

	// Replaces bootstrap config with production config
	configName := ConfigName(cfg.Cluster.Name)
	if err := system.WriteDnsmasqConfig(ctx, configName, content); err != nil {
		return utils.WrapError("failed to write dnsmasq config", err)
	}

	if err := system.RestartDnsmasq(ctx); err != nil {
		return utils.WrapError("failed to restart dnsmasq", err)
	}

	return nil
}

