package setup

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/templates"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// BuildHAProxyConfigData builds the HAProxy configuration data from node list.
func (p *Phase) BuildHAProxyConfigData(cfg *config.Config) (templates.HAProxyConfigData, error) {
	nodes, err := p.BuildNodeList(cfg)
	if err != nil {
		return templates.HAProxyConfigData{}, fmt.Errorf("failed to build node list: %w", err)
	}

	var masterServers, workerServers []templates.HAProxyServer
	var bootstrapIP string

	for _, node := range nodes {
		switch node.Role {
		case "bootstrap":
			bootstrapIP = node.IP
		case "master":
			masterServers = append(masterServers, templates.HAProxyServer{Name: node.Name, IP: node.IP})
		case "worker":
			workerServers = append(workerServers, templates.HAProxyServer{Name: node.Name, IP: node.IP})
		}
	}

	var backupServers []templates.HAProxyServer

	if len(workerServers) == 0 {
		// With no dedicated workers, masters handle ingress directly
		workerServers = masterServers
	} else {
		// Workers are configured but may not join; masters serve as backup for http/https
		backupServers = masterServers
	}

	return templates.HAProxyConfigData{
		ClusterDomain: fmt.Sprintf("%s.%s", cfg.Cluster.Name, cfg.Cluster.Domain),
		BootstrapIP:   bootstrapIP,
		MasterServers: masterServers,
		WorkerServers: workerServers,
		BackupServers: backupServers,
	}, nil
}

// writeHAProxyConfigToTemp writes HAProxy config content to a temporary file.
func writeHAProxyConfigToTemp(content string) (string, error) {
	tmpFile, err := os.CreateTemp("", "haproxy-*.cfg")
	if err != nil {
		return "", err
	}
	defer func() { _ = tmpFile.Close() }()

	if _, err := tmpFile.WriteString(content); err != nil {
		return "", err
	}
	return tmpFile.Name(), nil
}

// installHAProxyConfig copies the config file to /etc/haproxy and validates it.
func (p *Phase) installHAProxyConfig(ctx context.Context, tmpPath string) error {
	haproxyConfig := "/etc/haproxy/haproxy.cfg"

	if system.FileExists(haproxyConfig) {
		if err := system.CopyFileWithElevation(ctx, haproxyConfig, haproxyConfig+".backup", "haproxy.cfg backup"); err != nil {
			p.LogWarn("haproxy: could not backup existing haproxy.cfg")
		}
	}

	if err := system.CopyFileWithElevation(ctx, tmpPath, haproxyConfig, "haproxy config"); err != nil {
		return fmt.Errorf("failed to install haproxy config: %w", err)
	}

	result, _ := p.Exec.Run(ctx, "sudo", "haproxy", "-c", "-f", haproxyConfig)
	if result == nil || result.ExitCode != 0 {
		stderr := ""
		if result != nil {
			stderr = result.Stderr
		}
		return fmt.Errorf("haproxy configuration validation failed: %s", stderr)
	}
	return nil
}

// enableAndRestartHAProxy enables HAProxy on boot and restarts the service.
func enableAndRestartHAProxy(ctx context.Context) error {
	if err := system.ManageService(ctx, system.ServiceEnable, "haproxy", "haproxy load balancer"); err != nil {
		return err
	}
	return system.ManageService(ctx, system.ServiceRestart, "haproxy", "haproxy load balancer")
}

// ConfigureHAProxy generates and applies HAProxy configuration.
func (p *Phase) ConfigureHAProxy(ctx context.Context, cfg *config.Config, opts Options) error {
	data, err := p.BuildHAProxyConfigData(cfg)
	if err != nil {
		return fmt.Errorf("failed to build HAProxy config data: %w", err)
	}

	content, err := templates.RenderHAProxyConfig(data)
	if err != nil {
		return fmt.Errorf("failed to render haproxy.cfg template: %w", err)
	}

	tmpPath, err := writeHAProxyConfigToTemp(content)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmpPath) }()

	if err := p.installHAProxyConfig(ctx, tmpPath); err != nil {
		return err
	}

	if err := enableAndRestartHAProxy(ctx); err != nil {
		return err
	}

	p.LogInfo("haproxy: configuration validated, service enabled and restarted")
	return nil
}

// VerifyHAProxyPorts checks that HAProxy is listening on all required ports.
func (p *Phase) VerifyHAProxyPorts(ctx context.Context) error {
	ports := []struct {
		port        string
		description string
	}{
		{"6443", "Kubernetes API"},
		{"22623", "Machine Config Server"},
		{"80", "HTTP ingress"},
		{"443", "HTTPS ingress"},
	}

	result, err := p.Exec.Run(ctx, "ss", "-tlnp")
	if err != nil {
		return fmt.Errorf("failed to check listening ports: %w", err)
	}

	allListening := true
	for _, portInfo := range ports {
		pattern := fmt.Sprintf(":%s ", portInfo.port)
		if strings.Contains(result.Stdout, pattern) {
			p.LogInfo(fmt.Sprintf("haproxy: listening on port %s (%s)", portInfo.port, portInfo.description))
		} else {
			p.LogWarn(fmt.Sprintf("haproxy: may not be listening on port %s (%s)", portInfo.port, portInfo.description))
			allListening = false
		}
	}

	if !allListening {
		return fmt.Errorf("haproxy is not listening on all required ports")
	}

	return nil
}
