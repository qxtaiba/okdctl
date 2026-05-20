package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

func minimalCfg() *config.Config {
	return &config.Config{
		Cluster: config.ClusterConfig{
			Name:   "test",
			Domain: "example.com",
		},
		Topology: config.TopologyConfig{
			ControlPlane: config.NodeConfig{Count: 1},
		},
		Networking: config.NetworkingConfig{
			StaticIP: config.StaticIPConfig{Start: "192.168.1.10"},
		},
	}
}

func writeFakeHAProxy(t *testing.T, dir string, exitCode int) {
	t.Helper()
	script := filepath.Join(dir, "haproxy")
	body := "#!/bin/sh\nexit "
	if exitCode == 0 {
		body += "0\n"
	} else {
		body += "1\n"
	}
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake haproxy: %v", err)
	}
}

func setupSeams(t *testing.T, tmpDir string, restartFn func(context.Context) error) {
	t.Helper()
	origCfg := haproxyConfigPath
	origBak := haproxyBackupPath
	origEnable := enableAndRestartHAProxyFn
	t.Cleanup(func() {
		haproxyConfigPath = origCfg
		haproxyBackupPath = origBak
		enableAndRestartHAProxyFn = origEnable
	})
	haproxyConfigPath = filepath.Join(tmpDir, "haproxy.cfg")
	haproxyBackupPath = filepath.Join(tmpDir, "haproxy.cfg.backup")
	enableAndRestartHAProxyFn = restartFn
}

func newPhase(t *testing.T, binDir string) *Phase {
	t.Helper()
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	exec := executor.New(executor.WithInheritedEnv())
	return New("test", phase.WithExecutor(exec), phase.WithLogger(logutil.NopLogger))
}

func TestConfigureHAProxy_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	binDir := t.TempDir()
	writeFakeHAProxy(t, binDir, 0)

	setupSeams(t, tmpDir, func(_ context.Context) error { return nil })

	p := newPhase(t, binDir)
	if err := p.ConfigureHAProxy(context.Background(), minimalCfg(), &Options{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(haproxyConfigPath)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("config mode = %04o; want 0644", perm)
	}

	if _, err := os.Stat(haproxyBackupPath); err == nil {
		t.Error("backup must not exist when no prior config was present")
	}
}

func TestConfigureHAProxy_HappyPath_BackupCreated(t *testing.T) {
	tmpDir := t.TempDir()
	binDir := t.TempDir()
	writeFakeHAProxy(t, binDir, 0)

	setupSeams(t, tmpDir, func(_ context.Context) error { return nil })

	if err := os.WriteFile(haproxyConfigPath, []byte("# old config"), 0o644); err != nil {
		t.Fatalf("pre-seed config: %v", err)
	}

	p := newPhase(t, binDir)
	if err := p.ConfigureHAProxy(context.Background(), minimalCfg(), &Options{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(haproxyBackupPath); err != nil {
		t.Errorf("backup not created: %v", err)
	}
	info, err := os.Stat(haproxyConfigPath)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("config mode = %04o; want 0644", perm)
	}
}

func TestConfigureHAProxy_NoBackup_SkipsRollback(t *testing.T) {
	tmpDir := t.TempDir()
	binDir := t.TempDir()
	writeFakeHAProxy(t, binDir, 0)

	errRestart := errors.New("restart failed")
	setupSeams(t, tmpDir, func(_ context.Context) error { return errRestart })

	p := newPhase(t, binDir)
	err := p.ConfigureHAProxy(context.Background(), minimalCfg(), &Options{})
	if err == nil {
		t.Fatal("expected error from restart failure; got nil")
	}
	if !errors.Is(err, errRestart) {
		t.Errorf("errors.Is(err, errRestart) = false; got: %v", err)
	}

	if _, statErr := os.Stat(haproxyBackupPath); statErr == nil {
		t.Error("backup must not be created when no prior config exists")
	}
}
