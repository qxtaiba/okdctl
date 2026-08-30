package setup

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/provision"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// BuildHAProxyConfigData assembles HAProxy template data from cfg's node
// list; masters serve ingress directly when no workers are configured.
func (p *Phase) BuildHAProxyConfigData(cfg *config.Config) (templates.HAProxyConfigData, error) {
	nodes, err := provision.BuildNodeList(cfg)
	if err != nil {
		return templates.HAProxyConfigData{}, &errtypes.ConfigError{Msg: "build node list", Err: err}
	}

	var masterServers, workerServers []templates.HAProxyServer
	var bootstrapIP string

	for _, node := range nodes {
		switch node.Role {
		case nodetypes.RoleBootstrap:
			bootstrapIP = node.IP
		case nodetypes.RoleMaster:
			masterServers = append(masterServers, templates.HAProxyServer{Name: node.Name, IP: node.IP})
		case nodetypes.RoleWorker:
			workerServers = append(workerServers, templates.HAProxyServer{Name: node.Name, IP: node.IP})
		default:
			return templates.HAProxyConfigData{}, &errtypes.ConfigError{Msg: fmt.Sprintf("unexpected node role %q in node %q — HAProxy backend unrenderable", node.Role, node.Name)}
		}
	}

	var backupServers []templates.HAProxyServer

	if len(workerServers) == 0 {
		workerServers = masterServers
	} else {
		// Workers may not join yet; masters back up http/https meanwhile.
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

var (
	haproxyConfigPath = phase.DefaultHAProxyConfigPath
	haproxyBackupPath = phase.DefaultHAProxyBackupPath
)

func (p *Phase) installHAProxyConfig(ctx context.Context, tmpPath string) error {
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return &errtypes.ClusterError{Msg: "read temp haproxy config", Err: err}
	}
	if err := system.AtomicWriteString(haproxyConfigPath, string(data), 0o644); err != nil {
		return &errtypes.ClusterError{Msg: "install haproxy config", Err: err}
	}

	if _, err := p.Exec.RunChecked(ctx, "haproxy", "-c", "-f", haproxyConfigPath); err != nil {
		return &errtypes.ClusterError{Msg: "haproxy configuration validation failed", Err: err}
	}
	return nil
}

// enableAndRestartHAProxyFn lets tests substitute this step since
// system.ManageService errors on non-Linux.
var enableAndRestartHAProxyFn = enableAndRestartHAProxy

func enableAndRestartHAProxy(ctx context.Context) error {
	if err := system.ManageService(ctx, system.ServiceEnable, "haproxy"); err != nil {
		return err
	}
	return system.ManageService(ctx, system.ServiceRestart, "haproxy")
}

// ConfigureHAProxy renders, installs, and restarts haproxy.cfg; on failure it
// restores the previous config and restarts with that instead.
func (p *Phase) ConfigureHAProxy(ctx context.Context, cfg *config.Config, _ *Options) error {
	data, err := p.BuildHAProxyConfigData(cfg)
	if err != nil {
		return &errtypes.ConfigError{Msg: "build HAProxy config data", Err: err}
	}

	content, err := templates.RenderHAProxyConfig(&data)
	if err != nil {
		return &errtypes.ConfigError{Msg: "render haproxy.cfg template", Err: err}
	}

	// Final install runs under sudo; this write must target a user-writable
	// temp file, not /etc/haproxy directly.
	tmpPath, err := system.WriteTempFile("haproxy-*.cfg", 0o644, func(f *os.File) error {
		_, werr := f.WriteString(content)
		return werr
	})
	if err != nil {
		return &errtypes.ConfigError{Msg: "write temp haproxy config", Err: err}
	}
	defer func() { _ = os.Remove(tmpPath) }()

	// haproxyBackupPath holds the pristine pre-okdctl config, written once and
	// never overwritten again; postinstall's timestamped backups cover later
	// snapshots.
	hasBackup := system.FileExists(haproxyBackupPath)
	if !hasBackup && system.FileExists(haproxyConfigPath) {
		if err := system.CopyFile(haproxyConfigPath, haproxyBackupPath); err != nil {
			return &errtypes.ConfigError{Msg: "back up existing haproxy.cfg", Err: err}
		}
		hasBackup = true
	}

	restartFn := func() error {
		return system.ManageService(ctx, system.ServiceRestart, "haproxy")
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

	if err := enableAndRestartHAProxyFn(ctx); err != nil {
		return rollback("service restart failed", err)
	}

	p.Log.Info("haproxy: configuration validated, service enabled and restarted")
	return nil
}

// attemptHAProxyRollback restores cfgPath from backupPath and restarts via
// restartFn, joining any rollback failure onto cause (cause alone on success).
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

// VerifyHAProxyPorts dials the API, machine-config, HTTP, and HTTPS ports on
// 127.0.0.1, warning (not erroring) on any not yet listening.
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

	dialer := net.Dialer{Timeout: 2 * time.Second}
	for _, portInfo := range ports {
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", portInfo.port))
		if err != nil {
			p.Log.Warn("haproxy: may not be listening", "port", portInfo.port, "desc", portInfo.description)
			continue
		}
		_ = conn.Close()
		p.Log.Debug("haproxy: listening", "port", portInfo.port, "desc", portInfo.description)
	}

	return nil
}
