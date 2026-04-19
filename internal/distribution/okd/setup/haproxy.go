package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/system"
)

// BuildHAProxyConfigData assembles the HAProxy template data from cfg's
// node list. When no workers are configured, masters serve ingress directly.
func (p *Phase) BuildHAProxyConfigData(cfg *config.Config) (templates.HAProxyConfigData, error) {
	nodes, err := p.BuildNodeList(cfg)
	if err != nil {
		return templates.HAProxyConfigData{}, fmt.Errorf("failed to build node list: %w", err)
	}

	var masterServers, workerServers []templates.HAProxyServer
	var bootstrapIP string

	for _, node := range nodes {
		switch node.Role {
		case phase.RoleBootstrap:
			bootstrapIP = node.IP
		case phase.RoleMaster:
			masterServers = append(masterServers, templates.HAProxyServer{Name: node.Name, IP: node.IP})
		case phase.RoleWorker:
			workerServers = append(workerServers, templates.HAProxyServer{Name: node.Name, IP: node.IP})
		default:
			return templates.HAProxyConfigData{}, fmt.Errorf("unexpected node role %q in node %q — HAProxy backend unrenderable", node.Role, node.Name)
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

const (
	haproxyConfigPath = phase.DefaultHAProxyConfigPath
	haproxyBackupPath = phase.DefaultHAProxyBackupPath
)

func (p *Phase) installHAProxyConfig(ctx context.Context, tmpPath string) error {
	if err := system.CopyFile(tmpPath, haproxyConfigPath); err != nil {
		return fmt.Errorf("failed to install haproxy config: %w", err)
	}

	if _, err := p.Exec.RunChecked(ctx, "haproxy", "-c", "-f", haproxyConfigPath); err != nil {
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

// ConfigureHAProxy renders haproxy.cfg, installs it, validates with
// "haproxy -c", and restarts the service. On any failure the previous
// config is restored and haproxy is restarted with it.
func (p *Phase) ConfigureHAProxy(ctx context.Context, cfg *config.Config, _ *Options) error {
	data, err := p.BuildHAProxyConfigData(cfg)
	if err != nil {
		return fmt.Errorf("failed to build HAProxy config data: %w", err)
	}

	content, err := templates.RenderHAProxyConfig(&data)
	if err != nil {
		return fmt.Errorf("failed to render haproxy.cfg template: %w", err)
	}

	// A user-writable temp file is required because the final install step
	// runs under sudo, so the write here cannot target /etc/haproxy directly.
	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("haproxy-%d.cfg", os.Getpid()))
	if err := system.AtomicWriteString(tmpPath, content, 0o644); err != nil {
		return fmt.Errorf("failed to write temp haproxy config: %w", err)
	}
	defer func() { _ = os.Remove(tmpPath) }()

	// Back up the live config so we can roll back if validation or restart
	// fails after the new config is already in place.
	hasBackup := false
	if system.FileExists(haproxyConfigPath) {
		if err := system.CopyFile(haproxyConfigPath, haproxyBackupPath); err != nil {
			return fmt.Errorf("failed to back up existing haproxy.cfg: %w", err)
		}
		hasBackup = true
	}

	rollback := func(reason string, cause error) error {
		if !hasBackup {
			return cause
		}
		p.Log.Warn(fmt.Sprintf("haproxy: %s, restoring from backup", reason))
		if restoreErr := system.CopyFile(haproxyBackupPath, haproxyConfigPath); restoreErr != nil {
			return errors.Join(cause, fmt.Errorf("rollback restore failed: %w", restoreErr))
		}
		if chmodErr := os.Chmod(haproxyConfigPath, 0o644); chmodErr != nil {
			p.Log.Warn("haproxy: rollback chmod failed", "err", chmodErr)
		}
		// Restart with the old config so the node isn't left serving the
		// rejected one.
		if restartErr := system.ManageService(ctx, system.ServiceRestart, "haproxy", "haproxy load balancer"); restartErr != nil {
			p.Log.Warn("haproxy: rollback restart failed", "err", restartErr)
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

// VerifyHAProxyPorts checks that haproxy is listening on the API, machine
// config, HTTP, and HTTPS ports. Missing ports are logged as warnings but do
// not return an error — listeners can come up shortly after service start.
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
		p.Log.Warn("haproxy: failed to check listening ports", "err", err)
		return nil
	}

	for _, portInfo := range ports {
		pattern := fmt.Sprintf(":%s ", portInfo.port)
		if strings.Contains(result.Stdout, pattern) {
			p.Log.Info("haproxy: listening", "port", portInfo.port, "desc", portInfo.description)
		} else {
			p.Log.Warn("haproxy: may not be listening", "port", portInfo.port, "desc", portInfo.description)
		}
	}

	return nil
}
