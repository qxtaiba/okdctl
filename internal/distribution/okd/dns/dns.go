// Package dns provides dnsmasq configuration for OKD clusters.
package dns

import (
	"context"
	"errors"
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
		KubeVipIP:     kubeVipIP,
		UpstreamDNS:   cfg.Networking.DNS,
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

func ConfigName(clusterName string) string {
	return fmt.Sprintf("okd-%s", clusterName)
}

func Setup(ctx context.Context, fallbackDNS []string, logger utils.Logger) error {
	if err := system.EnableDnsmasq(ctx); err != nil {
		return utils.WrapError("failed to enable dnsmasq", err)
	}
	if err := system.ConfigureSystemResolver(ctx, fallbackDNS, logger); err != nil {
		return utils.WrapError("failed to configure system resolver", err)
	}
	return nil
}

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

	if err := validateAndRestartDnsmasq(ctx, configName); err != nil {
		return err
	}

	return nil
}

func DeployProduction(ctx context.Context, cfg *config.Config, appsIP, kubeVipIP string, customDomains []templates.DNSCustomDomain) error {
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
	for _, cd := range customDomains {
		if net.ParseIP(cd.IP) == nil {
			return fmt.Errorf("invalid custom domain IP address for %s: %s", cd.Domain, cd.IP)
		}
	}
	data.AppsIP = appsIP
	data.KubeVipIP = kubeVipIP
	data.CustomDomains = customDomains

	content, err := templates.RenderDNSProductionConfig(data)
	if err != nil {
		return utils.WrapError("failed to render production dns config", err)
	}

	configName := ConfigName(cfg.Cluster.Name)
	if err := system.WriteDnsmasqConfig(ctx, configName, content); err != nil {
		return utils.WrapError("failed to write dnsmasq config", err)
	}

	if err := validateAndRestartDnsmasq(ctx, configName); err != nil {
		return err
	}

	return nil
}

// validateAndRestartDnsmasq validates the dnsmasq config, then restarts the service.
// On validation or restart failure, it restores the previous config from the .backup file.
func validateAndRestartDnsmasq(ctx context.Context, configName string) error {
	configPath := system.DnsmasqConfigPath(configName)
	backupPath := configPath + ".backup"

	restore := func() {
		if !system.FileExists(backupPath) {
			return
		}
		_ = system.CopyFileWithElevation(ctx, backupPath, configPath, "dnsmasq config rollback")
	}

	if err := system.ValidateDnsmasqConfig(ctx); err != nil {
		restore()
		return errors.Join(
			fmt.Errorf("dnsmasq config validation failed — previous config restored"),
			err,
		)
	}

	if err := system.RestartDnsmasq(ctx); err != nil {
		restore()
		return utils.WrapError("failed to restart dnsmasq — previous config restored", err)
	}

	// Successful restart — clean up backup.
	if system.FileExists(backupPath) {
		_ = system.RemoveAll(ctx, backupPath, "dnsmasq config backup")
	}

	return nil
}

