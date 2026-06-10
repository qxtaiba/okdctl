package okd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

func seedTFState(t *testing.T, projectRoot, env string) {
	t.Helper()
	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", env)
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"version":4,"resources":[{"type":"x"}]}`)
	if err := os.WriteFile(filepath.Join(envDir, "terraform.tfstate"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func seedAuth(t *testing.T, workDir string) {
	t.Helper()
	authDir := filepath.Join(workDir, "cluster-config", "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestGuardLiveCluster(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, projectRoot, workDir string)
		opts    PrepareOpts
		wantErr bool
	}{
		{
			name:    "no state no auth no flag",
			setup:   func(_ *testing.T, _, _ string) {},
			opts:    PrepareOpts{},
			wantErr: false,
		},
		{
			name: "tfstate present no flag",
			setup: func(t *testing.T, projectRoot, _ string) {
				seedTFState(t, projectRoot, "production")
			},
			opts:    PrepareOpts{},
			wantErr: true,
		},
		{
			name: "auth present no flag",
			setup: func(t *testing.T, _, workDir string) {
				seedAuth(t, workDir)
			},
			opts:    PrepareOpts{},
			wantErr: true,
		},
		{
			name: "tfstate present fresh bypass",
			setup: func(t *testing.T, projectRoot, _ string) {
				seedTFState(t, projectRoot, "production")
			},
			opts:    PrepareOpts{FreshDeploy: true},
			wantErr: false,
		},
		{
			name: "auth present resume bypass",
			setup: func(t *testing.T, _, workDir string) {
				seedAuth(t, workDir)
			},
			opts:    PrepareOpts{ResumeInProgress: true},
			wantErr: false,
		},
		{
			name: "both present resume bypass",
			setup: func(t *testing.T, projectRoot, workDir string) {
				seedTFState(t, projectRoot, "production")
				seedAuth(t, workDir)
			},
			opts:    PrepareOpts{ResumeInProgress: true},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			workDir := filepath.Join(root, "okd-install")
			if err := os.MkdirAll(workDir, 0o755); err != nil {
				t.Fatal(err)
			}
			tc.setup(t, root, workDir)

			p := New("test", WithProjectRoot(root), WithLogger(logutil.NopLogger))
			err := p.guardLiveCluster(config.DefaultConfig(), workDir, tc.opts)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error; got nil")
				}
				var cfgErr *errtypes.ConfigError
				if !errors.As(err, &cfgErr) {
					t.Errorf("err = %v; want *errtypes.ConfigError", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil; got %v", err)
			}
		})
	}
}
