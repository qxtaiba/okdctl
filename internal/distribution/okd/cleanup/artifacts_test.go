package cleanup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

func TestRefuseCriticalPath(t *testing.T) {
	critical := []string{
		"/", "/etc", "/var", "/usr", "/bin", "/sbin", "/lib", "/home",
		"/root", "/boot", "/dev", "/proc", "/sys",
	}
	for _, p := range critical {
		if err := refuseCriticalPath(p); err == nil {
			t.Errorf("refuseCriticalPath(%q) allowed; want rejection", p)
		}
	}

	// filepath.Clean normalizes "/etc/" → "/etc", so the trailing-slash form
	// must also be rejected.
	cleanedToCritical := []string{"/etc/", "/var/", "/../etc"}
	for _, p := range cleanedToCritical {
		if err := refuseCriticalPath(p); err == nil {
			t.Errorf("refuseCriticalPath(%q) allowed after Clean; want rejection", p)
		}
	}

	safe := []string{
		"/tmp/okdctl-work",
		"/var/lib/okdctl",
		"/home/alice/.okdctl",
		"/etc/okdctl.d/cfg",
	}
	for _, p := range safe {
		if err := refuseCriticalPath(p); err != nil {
			t.Errorf("refuseCriticalPath(%q) rejected safe path: %v", p, err)
		}
	}
}

func TestSafeRemoveWithLogger(t *testing.T) {
	ctx := context.Background()
	logger := logutil.NopLogger

	t.Run("missing path returns nil (not an error)", func(t *testing.T) {
		dir := t.TempDir()
		if err := SafeRemoveWithLogger(ctx, filepath.Join(dir, "never-existed"), "test", logger); err != nil {
			t.Errorf("expected nil for missing path; got %v", err)
		}
	})

	t.Run("removes regular file", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "f")
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := SafeRemoveWithLogger(ctx, p, "file", logger); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("file still present: %v", err)
		}
	})

	t.Run("removes directory tree recursively", func(t *testing.T) {
		dir := t.TempDir()
		sub := filepath.Join(dir, "sub")
		nested := filepath.Join(sub, "nested")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(nested, "a"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}

		if err := SafeRemoveWithLogger(ctx, sub, "subtree", logger); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(sub); !os.IsNotExist(err) {
			t.Errorf("subtree still present: %v", err)
		}
	})

	t.Run("refuses critical system path", func(t *testing.T) {
		err := SafeRemoveWithLogger(ctx, "/etc", "sysetc", logger)
		if err == nil {
			t.Fatal("expected rejection for /etc")
		}
	})

	t.Run("nil logger does not panic", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "f")
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := SafeRemoveWithLogger(ctx, p, "file", nil); err != nil {
			t.Fatal(err)
		}
	})
}

func TestWorkDirectory_PreservesConfigYaml(t *testing.T) {
	workDir := t.TempDir()
	ctx := context.Background()

	configFile := filepath.Join(workDir, "okdctl.yaml")
	if err := os.WriteFile(configFile, []byte("cluster: test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	subTrees := []string{"tmp", "downloads", "installer", "custom-isos"}
	for _, sub := range subTrees {
		if err := os.MkdirAll(filepath.Join(workDir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := WorkDirectory(ctx, workDir, true, logutil.NopLogger); err != nil {
		t.Fatalf("WorkDirectory returned error: %v", err)
	}

	if _, err := os.Stat(configFile); err != nil {
		t.Errorf("okdctl.yaml was removed; want it preserved: %v", err)
	}

	for _, sub := range subTrees {
		p := filepath.Join(workDir, sub)
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("sub-tree %q still present after cleanup; want removed", sub)
		}
	}
}
