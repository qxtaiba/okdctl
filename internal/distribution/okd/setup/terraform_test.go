package setup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

func TestGenerateTerraformVars_RemovesBootstrapSentinel(t *testing.T) {
	root := t.TempDir()
	cfg := config.DefaultConfig()
	envDir := workspace.TerraformEnvDir(root, cfg.TerraformEnvName())
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(envDir, workspace.BootstrapStateSentinelFile)
	if err := os.WriteFile(sentinel, []byte(`{"bootstrap_enabled": false}`), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := &Options{BaseOptions: phase.BaseOptions{ProjectRoot: root}}
	if err := (&Phase{}).GenerateTerraformVars(context.Background(), cfg, opts); err != nil {
		t.Fatalf("GenerateTerraformVars: %v", err)
	}

	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Errorf("bootstrap sentinel must be removed by GenerateTerraformVars, stat err: %v", err)
	}
	fi, err := os.Stat(filepath.Join(envDir, "terraform.tfvars"))
	if err != nil {
		t.Fatalf("terraform.tfvars not rendered: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("terraform.tfvars perm = %#o, want 0600", perm)
	}
}

func TestGenerateTerraformVars_RequiresProxmoxProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox = nil

	opts := &Options{BaseOptions: phase.BaseOptions{ProjectRoot: t.TempDir()}}
	if err := (&Phase{}).GenerateTerraformVars(context.Background(), cfg, opts); err == nil {
		t.Error("GenerateTerraformVars without proxmox provider: want error, got nil")
	}
}
