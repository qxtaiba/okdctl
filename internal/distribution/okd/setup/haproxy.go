package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// BuildHAProxyConfigData assembles the HAProxy template data from cfg's
// node list. When no workers are configured, masters serve ingress directly.
func (p *Phase) BuildHAProxyConfigData(cfg *config.Config) (templates.HAProxyConfigData, error) {
	nodes, err := p.BuildNodeList(cfg)
	if err != nil {
		return templates.HAProxyConfigData{}, &errtypes.ConfigError{Msg: "failed to build node list", Err: err}
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
			return templates.HAProxyConfigData{}, &errtypes.ConfigError{Msg: fmt.Sprintf("unexpected node role %q in node %q — HAProxy backend unrenderable", node.Role, node.Name)}
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
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return &errtypes.ClusterError{Msg: "failed to read temp haproxy config", Err: err}
	}
	if err := system.AtomicWriteString(haproxyConfigPath, string(data), 0o644); err != nil {
		return &errtypes.ClusterError{Msg: "failed to install haproxy config", Err: err}
	}

	if _, err := p.Exec.RunChecked(ctx, "haproxy", "-c", "-f", haproxyConfigPath); err != nil {
		return &errtypes.ClusterError{Msg: "haproxy configuration validation failed", Err: err}
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
		return &errtypes.ConfigError{Msg: "failed to build HAProxy config data", Err: err}
	}

	content, err := templates.RenderHAProxyConfig(&data)
	if err != nil {
		return &errtypes.ConfigError{Msg: "failed to render haproxy.cfg template", Err: err}
	}

	// A user-writable temp file is required because the final install step
	// runs under sudo, so the write here cannot target /etc/haproxy directly.
	tmpPath, err := system.WriteTempFile("haproxy-*.cfg", 0o644, func(f *os.File) error {
		_, werr := f.WriteString(content)
		return werr
	})
	if err != nil {
		return &errtypes.ConfigError{Msg: "failed to write temp haproxy config", Err: err}
	}
	defer func() { _ = os.Remove(tmpPath) }()

	// Back up the live config so we can roll back if validation or restart
	// fails after the new config is already in place.
	hasBackup := false
	if system.FileExists(haproxyConfigPath) {
		if err := system.CopyFile(haproxyConfigPath, haproxyBackupPath); err != nil {
			return &errtypes.ConfigError{Msg: "failed to back up existing haproxy.cfg", Err: err}
		}
		hasBackup = true
	}

	restartFn := func() error {
		return system.ManageService(ctx, system.ServiceRestart, "haproxy", "haproxy load balancer")
	}
	rollback := func(reason string, cause error) error {
		if !hasBackup {
			return cause
		}
		p.Log.Warn("haproxy: restoring from backup", "reason", reason)
		return attemptHAProxyRollback(cause, haproxyConfigPath, haproxyBackupPath, system.AtomicWriteString, restartFn)
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

// attemptHAProxyRollback reads backupPath, writes its content to cfgPath via
// writeFn, then calls restartFn. On any failure cause is joined with the
// rollback error; on success only cause is returned.
func attemptHAProxyRollback(
	cause error,
	cfgPath, backupPath string,
	writeFn func(string, string, os.FileMode) error,
	restartFn func() error,
) error {
	backupData, readErr := os.ReadFile(backupPath)
	if readErr != nil {
		return errors.Join(cause, fmt.Errorf("rollback read backup failed: %w", readErr))
	}
	if restoreErr := writeFn(cfgPath, string(backupData), 0o644); restoreErr != nil {
		return errors.Join(cause, fmt.Errorf("rollback restore failed: %w", restoreErr))
	}
	if restartErr := restartFn(); restartErr != nil {
		return errors.Join(cause, fmt.Errorf("rollback restart failed: %w", restartErr))
	}
	return cause
}

// VerifyHAProxyPorts checks that haproxy is listening on the API, machine
// config, HTTP, and HTTPS ports. Missing ports are logged as warnings but do
// not return an error — listeners can come up shortly after service start.
func (p *Phase) VerifyHAProxyPorts(ctx context.Context) error {
	ports := []struct {
		port        string
		description string
	}{
		{strconv.Itoa(phase.KubeAPIPort), "Kubernetes API"},
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
