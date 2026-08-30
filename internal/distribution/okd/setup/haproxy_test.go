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

func newHAProxyPhase(t *testing.T, restartFn func(context.Context) error) *Phase {
	t.Helper()
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "haproxy"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake haproxy: %v", err)
	}

	tmpDir := t.TempDir()
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

	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	exec := executor.New(executor.WithInheritedEnv())
	return New(phase.WithExecutor(exec), phase.WithLogger(logutil.NopLogger))
}

func TestConfigureHAProxy_HappyPath(t *testing.T) {
	p := newHAProxyPhase(t, func(_ context.Context) error { return nil })
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

func TestConfigureHAProxy_NoBackup_SkipsRollback(t *testing.T) {
	errRestart := errors.New("restart failed")
	p := newHAProxyPhase(t, func(_ context.Context) error { return errRestart })

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

func TestConfigureHAProxy_RerunPreservesPristineBackup(t *testing.T) {
	p := newHAProxyPhase(t, func(_ context.Context) error { return nil })

	const pristine = "# operator config"
	if err := os.WriteFile(haproxyConfigPath, []byte(pristine), 0o644); err != nil {
		t.Fatalf("pre-seed config: %v", err)
	}

	for run := 1; run <= 2; run++ {
		if err := p.ConfigureHAProxy(context.Background(), minimalCfg(), &Options{}); err != nil {
			t.Fatalf("run %d: unexpected error: %v", run, err)
		}
	}

	data, err := os.ReadFile(haproxyBackupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(data) != pristine {
		t.Errorf("pristine backup clobbered on re-run: got %q, want %q", data, pristine)
	}
}
