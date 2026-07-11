package install

import (
	"context"
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
	return &Phase{
		BasePhase: phase.NewBasePhase(
			phase.WithLogger(logutil.NopLogger),
			phase.WithExecutor(executor.New(executor.WithLogger(logutil.NopLogger))),
			phase.WithReporter(logutil.NopProgressReporter),
		),
	}
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

// seedKubeconfig writes content to clusterDir/auth/kubeconfig and returns clusterDir.
func seedKubeconfig(t *testing.T, content string) string {
	t.Helper()
	clusterDir := t.TempDir()
	authDir := filepath.Join(clusterDir, "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir auth: %v", err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "kubeconfig"), []byte(content), 0o600); err != nil {
		t.Fatalf("write src kubeconfig: %v", err)
	}
	return clusterDir
}

func TestSetupClusterAccess_FreshInstall(t *testing.T) {
	homeDir := t.TempDir()
	orig := invokingUserHomeDirFn
	invokingUserHomeDirFn = func() (string, error) { return homeDir, nil }
	t.Cleanup(func() { invokingUserHomeDirFn = orig })

	srcContent := "apiVersion: v1\nkind: Config\n"
	clusterDir := seedKubeconfig(t, srcContent)

	p := newInstallPhase(t)
	if err := p.SetupClusterAccess(context.Background(), clusterDir); err != nil {
		t.Fatalf("SetupClusterAccess: %v", err)
	}

	dest := filepath.Join(homeDir, ".kube", "config")
	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("dest not created: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("dest perm = %04o, want 0600", perm)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != srcContent {
		t.Errorf("dest content = %q, want %q", got, srcContent)
	}
}

func TestSetupClusterAccess_BackupOnOverwrite(t *testing.T) {
	homeDir := t.TempDir()
	orig := invokingUserHomeDirFn
	invokingUserHomeDirFn = func() (string, error) { return homeDir, nil }
	t.Cleanup(func() { invokingUserHomeDirFn = orig })

	kubeDir := filepath.Join(homeDir, ".kube")
	if err := os.MkdirAll(kubeDir, 0o755); err != nil {
		t.Fatalf("mkdir .kube: %v", err)
	}
	existingContent := "old-kubeconfig-content\n"
	dest := filepath.Join(kubeDir, "config")
	if err := os.WriteFile(dest, []byte(existingContent), 0o600); err != nil {
		t.Fatalf("seed existing config: %v", err)
	}

	newContent := "new-kubeconfig-content\n"
	clusterDir := seedKubeconfig(t, newContent)

	p := newInstallPhase(t)
	if err := p.SetupClusterAccess(context.Background(), clusterDir); err != nil {
		t.Fatalf("SetupClusterAccess: %v", err)
	}

	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("dest not present after overwrite: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("dest perm = %04o, want 0600", perm)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != newContent {
		t.Errorf("dest content = %q, want %q", got, newContent)
	}

	backups, err := filepath.Glob(dest + ".backup.*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("backup count = %d, want 1; entries: %v", len(backups), backups)
	}
	bfi, err := os.Stat(backups[0])
	if err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if perm := bfi.Mode().Perm(); perm != 0o600 {
		t.Errorf("backup perm = %04o, want 0600", perm)
	}
	backupBytes, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backupBytes) != existingContent {
		t.Errorf("backup content = %q, want %q", backupBytes, existingContent)
	}
}

func TestSetupClusterAccess_CtxCancelledLeavesDestUntouched(t *testing.T) {
	homeDir := t.TempDir()
	orig := invokingUserHomeDirFn
	invokingUserHomeDirFn = func() (string, error) { return homeDir, nil }
	t.Cleanup(func() { invokingUserHomeDirFn = orig })

	clusterDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := newInstallPhase(t)
	err := p.SetupClusterAccess(ctx, clusterDir)
	if err == nil {
		t.Fatal("expected error from cancelled ctx, got nil")
	}

	dest := filepath.Join(homeDir, ".kube", "config")
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("dest must not exist after ctx cancel; stat: %v", statErr)
	}
}
