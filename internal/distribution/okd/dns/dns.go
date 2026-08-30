// Package dns configures dnsmasq for OKD clusters.
// Some exports are public API despite only intra-package callers today.
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
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// buildConfigData assembles DNS template data, checking node IPs against
// machineCIDR up front to fail early.
func buildConfigData(cfg *config.Config) (templates.DNSConfigData, error) {
	if cfg == nil {
		return templates.DNSConfigData{}, &errtypes.ConfigError{Msg: "config cannot be nil"}
	}

	clusterDomain := fmt.Sprintf("%s.%s", cfg.Cluster.Name, cfg.Cluster.Domain)

	if cfg.Cluster.Name == "" {
		return templates.DNSConfigData{}, &errtypes.ConfigError{Msg: "cluster name is required"}
	}
	if !config.IsValidDNSLabel(cfg.Cluster.Name) {
		return templates.DNSConfigData{}, &errtypes.ConfigError{Msg: fmt.Sprintf("cluster name %q is not a valid DNS label", cfg.Cluster.Name)}
	}
	if cfg.Networking.Bastion.IP == "" {
		return templates.DNSConfigData{}, &errtypes.ConfigError{Msg: "bastion IP is required"}
	}
	if cfg.Networking.StaticIP.Start == "" {
		return templates.DNSConfigData{}, &errtypes.ConfigError{Msg: "static IP start is required"}
	}

	enum, err := nodetypes.ClusterNodes(cfg)
	if err != nil {
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

	for _, n := range enum {
		node := templates.DNSNode{
			Name: n.PrefixedName(cfg.Cluster.Name),
			IP:   n.IP,
		}
		switch n.Role {
		case nodetypes.RoleBootstrap:
			data.BootstrapNode = node
		case nodetypes.RoleMaster:
			data.MasterNodes = append(data.MasterNodes, node)
		case nodetypes.RoleWorker:
			data.WorkerNodes = append(data.WorkerNodes, node)
		}
	}

	return data, nil
}

// GenerateBootstrapConfig renders the bootstrap dnsmasq config to outputDir/dnsmasq-bootstrap.conf.
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

func configName(clusterName string) string {
	return fmt.Sprintf("okd-%s", clusterName)
}

// IsBootstrapDNS reports whether api.* in the on-disk config still points at
// the bastion IP (false if absent). Matching is line-exact, not substring, to
// avoid IP-suffix aliasing (10.0.0.10 vs 10.0.0.1).
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

// Setup enables dnsmasq and points the system resolver at it, falling back to
// fallbackDNS when the cluster resolver is unavailable.
func Setup(ctx context.Context, fallbackDNS []string, logger *slog.Logger) error {
	if err := EnableDnsmasq(ctx); err != nil {
		return fmt.Errorf("enable dnsmasq: %w", err)
	}
	if err := ConfigureSystemResolver(ctx, fallbackDNS, logger); err != nil {
		return fmt.Errorf("configure system resolver: %w", err)
	}
	return nil
}

// DeployBootstrap writes the bootstrap dnsmasq config and restarts the service.
// On failure it restores the previous config.
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

	return validateAndRestartDnsmasq(ctx, cn)
}

// DeployProduction writes the post-install dnsmasq config (cluster apps/VIP
// records plus custom-domain overrides) and restarts dnsmasq. On failure it
// restores the previous config.
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

	return validateAndRestartDnsmasq(ctx, cn)
}

// recoveryRestartTimeout bounds the best-effort restart after a rollback.
const recoveryRestartTimeout = 30 * time.Second

// validateAndRestartDnsmasq restores .backup on failure, retrying the restart
// once more if the restart itself failed.
func validateAndRestartDnsmasq(ctx context.Context, configName string) error {
	configPath, err := DnsmasqConfigPath(configName)
	if err != nil {
		return err
	}
	backupPath := configPath + ".backup"

	// restore reverts to backup, or on first deploy (no backup) removes the
	// drop-in to avoid leaving broken root-owned config.
	restore := func() string {
		if system.FileExists(backupPath) {
			// No chmod: CopyFile preserves mode; chmod would follow symlinks.
			_ = system.CopyFile(backupPath, configPath)
			return "previous config restored"
		}
		_ = os.Remove(configPath)
		return "rejected config removed"
	}

	if err := validateDnsmasqConfigFn(ctx); err != nil {
		outcome := restore()
		return fmt.Errorf("dnsmasq config validation failed — %s: %w", outcome, err)
	}

	if err := restartDnsmasqFn(ctx); err != nil {
		outcome := restore()
		// Restart again so the service converges; ctx is detached so a Ctrl-C
		// killing the first restart can't kill this one too.
		rCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recoveryRestartTimeout)
		defer cancel()
		if rErr := restartDnsmasqFn(rCtx); rErr != nil {
			return fmt.Errorf("restart dnsmasq — %s on disk but dnsmasq restart with it also failed: %w", outcome, errors.Join(err, rErr))
		}
		return fmt.Errorf("restart dnsmasq — %s and service restarted with it: %w", outcome, err)
	}

	if system.FileExists(backupPath) {
		_ = os.RemoveAll(backupPath)
	}

	return nil
}
