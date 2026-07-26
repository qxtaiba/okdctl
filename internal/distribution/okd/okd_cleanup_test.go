package okd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// TestProvisionerCleanup locks the facade verb the CLI routes through: a
// WorkOnly cleanup with no terraform state removes the work directory.
func TestProvisionerCleanup(t *testing.T) {
	projectRoot := t.TempDir()
	workDir := filepath.Join(projectRoot, "okd-install")
	if err := os.MkdirAll(filepath.Join(workDir, "downloads"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	p := New(WithProjectRoot(projectRoot), WithLogger(logutil.NopLogger))
	opts := cleanup.NewOptions(cfg, projectRoot, cleanup.WorkOnly)

	if err := p.Cleanup(context.Background(), &opts); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Errorf("work directory not removed: %v", err)
	}
}
