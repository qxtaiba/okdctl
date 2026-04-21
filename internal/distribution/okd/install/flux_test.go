package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

func newInstallPhase(t *testing.T) *Phase {
	t.Helper()
	return &Phase{BasePhase: phase.NewBasePhase("test",
		phase.WithLogger(logutil.NopLogger),
		phase.WithExecutor(executor.New(executor.WithLogger(logutil.NopLogger))),
	)}
}

func TestAddKubeconfigToBashrc_Idempotent(t *testing.T) {
	homeDir := t.TempDir()
	bashrcPath := filepath.Join(homeDir, ".bashrc")
	original := "export KUBECONFIG=/old\n"
	if err := os.WriteFile(bashrcPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write .bashrc: %v", err)
	}

	p := newInstallPhase(t)
	if err := p.addKubeconfigToBashrc(homeDir, "/new/kubeconfig"); err != nil {
		t.Fatalf("addKubeconfigToBashrc: %v", err)
	}

	got, err := os.ReadFile(bashrcPath)
	if err != nil {
		t.Fatalf("read .bashrc: %v", err)
	}
	if string(got) != original {
		t.Errorf("bashrc modified; got %q, want %q", string(got), original)
	}
}

func TestAddKubeconfigToBashrc_PreservesMode(t *testing.T) {
	homeDir := t.TempDir()
	bashrcPath := filepath.Join(homeDir, ".bashrc")
	if err := os.WriteFile(bashrcPath, []byte("# existing\n"), 0o600); err != nil {
		t.Fatalf("write .bashrc: %v", err)
	}

	p := newInstallPhase(t)
	if err := p.addKubeconfigToBashrc(homeDir, filepath.Join(homeDir, ".kube", "config")); err != nil {
		t.Fatalf("addKubeconfigToBashrc: %v", err)
	}

	fi, err := os.Stat(bashrcPath)
	if err != nil {
		t.Fatalf("stat .bashrc: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf(".bashrc perm = %04o, want 0600", got)
	}
}

func TestAddKubeconfigToBashrc_CreatesIfMissing(t *testing.T) {
	homeDir := t.TempDir()
	kubeconfigPath := filepath.Join(homeDir, ".kube", "config")

	p := newInstallPhase(t)
	if err := p.addKubeconfigToBashrc(homeDir, kubeconfigPath); err != nil {
		t.Fatalf("addKubeconfigToBashrc: %v", err)
	}

	bashrcPath := filepath.Join(homeDir, ".bashrc")
	fi, err := os.Stat(bashrcPath)
	if err != nil {
		t.Fatalf(".bashrc not created: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o644 {
		t.Errorf(".bashrc perm = %04o, want 0644", got)
	}
	content, err := os.ReadFile(bashrcPath)
	if err != nil {
		t.Fatalf("read .bashrc: %v", err)
	}
	if !strings.Contains(string(content), "export KUBECONFIG=") {
		t.Errorf(".bashrc missing export line; content: %q", string(content))
	}
}
