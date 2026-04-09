package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/phase"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/templates"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

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

// writeHAProxyConfigToTemp writes the rendered haproxy.cfg contents to a
// PID-named file under os.TempDir using system.AtomicWrite. The caller
// is responsible for removing the returned path. A user-writable temp file is
// required because the final install step runs under sudo, so the write
// itself cannot target /etc/haproxy directly here.
func writeHAProxyConfigToTemp(content string) (string, error) {
	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("haproxy-%d.cfg", os.Getpid()))
	if err := system.AtomicWriteString(tmpPath, content, 0644); err != nil {
		return "", fmt.Errorf("failed to write temp haproxy config: %w", err)
	}
	return tmpPath, nil
}

const (
	haproxyConfigPath = phase.DefaultHAProxyConfigPath
	haproxyBackupPath = phase.DefaultHAProxyBackupPath
)

func (p *Phase) installHAProxyConfig(ctx context.Context, tmpPath string) error {
	if err := system.CopyFileWithElevation(ctx, tmpPath, haproxyConfigPath, "haproxy config"); err != nil {
		return fmt.Errorf("failed to install haproxy config: %w", err)
	}

	if _, err := p.Exec.RunChecked(ctx, "sudo", "haproxy", "-c", "-f", haproxyConfigPath); err != nil {
		return fmt.Errorf("haproxy configuration validation failed: %w", err)
	}
	return nil
}

func enableAndRestartHAProxy(ctx context.Context) error {
	if err := system.ManageService(ctx, system.ServiceEnable, "haproxy", "haproxy load balancer"); err != nil {
		return err
	}
	return system.ManageService(ctx, system.ServiceRestart, "haproxy", "haproxy load balancer")
}

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

	// Back up the live config so we can roll back if validation or restart
	// fails after the new config is already in place.
	hasBackup := false
	if system.FileExists(haproxyConfigPath) {
		if err := system.CopyFileWithElevation(ctx, haproxyConfigPath, haproxyBackupPath, "haproxy.cfg backup"); err != nil {
			return fmt.Errorf("failed to back up existing haproxy.cfg: %w", err)
		}
		hasBackup = true
	}

	rollback := func(reason string, cause error) error {
		if !hasBackup {
			return cause
		}
		p.Log.Warn(fmt.Sprintf("haproxy: %s, restoring from backup", reason))
		if restoreErr := system.CopyFileWithElevation(ctx, haproxyBackupPath, haproxyConfigPath, "haproxy.cfg rollback"); restoreErr != nil {
			return errors.Join(cause, fmt.Errorf("rollback restore failed: %w", restoreErr))
		}
		if chmodErr := system.Chmod(ctx, haproxyConfigPath, "644", "haproxy config rollback perms"); chmodErr != nil {
			p.Log.Warn(fmt.Sprintf("haproxy: rollback chmod failed: %v", chmodErr))
		}
		// Restart with the old config so the node isn't left serving the
		// rejected one.
		if restartErr := system.ManageService(ctx, system.ServiceRestart, "haproxy", "haproxy load balancer"); restartErr != nil {
			p.Log.Warn(fmt.Sprintf("haproxy: rollback restart failed: %v", restartErr))
			return errors.Join(cause, fmt.Errorf("rollback restart failed: %w", restartErr))
		}
		return cause
	}

	if err := p.installHAProxyConfig(ctx, tmpPath); err != nil {
		return rollback("config install or validation failed", err)
	}

	if err := enableAndRestartHAProxy(ctx); err != nil {
		return rollback("service restart failed", err)
	}

	p.Log.Info("haproxy: configuration validated, service enabled and restarted")
	return nil
}

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
		p.Log.Warn(fmt.Sprintf("haproxy: failed to check listening ports: %v", err))
		return nil
	}

	for _, portInfo := range ports {
		pattern := fmt.Sprintf(":%s ", portInfo.port)
		if strings.Contains(result.Stdout, pattern) {
			p.Log.Info(fmt.Sprintf("haproxy: listening on port %s (%s)", portInfo.port, portInfo.description))
		} else {
			p.Log.Warn(fmt.Sprintf("haproxy: may not be listening on port %s (%s)", portInfo.port, portInfo.description))
		}
	}

	return nil
}
