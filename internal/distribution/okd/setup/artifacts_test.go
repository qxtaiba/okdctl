package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/provision"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

func newTestPhase(t *testing.T) *Phase {
	t.Helper()
	return &Phase{Provisioner: provision.Provisioner{BasePhase: phase.NewBasePhase(
		phase.WithLogger(logutil.NopLogger),
		phase.WithExecutor(executor.New(executor.WithLogger(logutil.NopLogger))),
	)}}
}

func TestAtomicInstallFile(t *testing.T) {
	t.Run("installs with mode and content", func(t *testing.T) {
		srcDir, destDir := t.TempDir(), t.TempDir()
		src := filepath.Join(srcDir, "oc")
		if err := os.WriteFile(src, []byte("#!/bin/sh\necho oc\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		dst := filepath.Join(destDir, "oc")
		if err := atomicInstallFile(t.Context(), src, destDir, "oc"); err != nil {
			t.Fatalf("atomicInstallFile: %v", err)
		}

		data, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("read installed file: %v", err)
		}
		if string(data) != "#!/bin/sh\necho oc\n" {
			t.Errorf("installed content = %q, want source content", data)
		}
		info, err := os.Stat(dst)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o755 {
			t.Errorf("installed perm = %04o, want 0755", perm)
		}
	})

	t.Run("replaces existing file", func(t *testing.T) {
		srcDir, destDir := t.TempDir(), t.TempDir()
		src := filepath.Join(srcDir, "kubectl")
		if err := os.WriteFile(src, []byte("new"), 0o600); err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(destDir, "kubectl")
		if err := os.WriteFile(dst, []byte("old truncated"), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := atomicInstallFile(t.Context(), src, destDir, "kubectl"); err != nil {
			t.Fatalf("atomicInstallFile: %v", err)
		}
		data, err := os.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "new" {
			t.Errorf("installed content = %q, want %q", data, "new")
		}
	})

	t.Run("leaves no temp files behind", func(t *testing.T) {
		srcDir, destDir := t.TempDir(), t.TempDir()
		src := filepath.Join(srcDir, "openshift-install")
		if err := os.WriteFile(src, []byte("bin"), 0o600); err != nil {
			t.Fatal(err)
		}

		if err := atomicInstallFile(t.Context(), src, destDir, "openshift-install"); err != nil {
			t.Fatalf("atomicInstallFile: %v", err)
		}
		entries, err := os.ReadDir(destDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Name() != "openshift-install" {
			t.Errorf("destDir entries = %v, want only openshift-install", entries)
		}
	})

	t.Run("missing source errors without touching dst", func(t *testing.T) {
		destDir := t.TempDir()
		dst := filepath.Join(destDir, "oc")
		if err := atomicInstallFile(t.Context(), filepath.Join(t.TempDir(), "absent"), destDir, "oc"); err == nil {
			t.Fatal("want error for missing source, got nil")
		}
		if _, err := os.Stat(dst); err == nil {
			t.Error("dst must not exist after failed install")
		}
	})

	t.Run("rejects names that escape destDir", func(t *testing.T) {
		srcDir, destDir := t.TempDir(), t.TempDir()
		src := filepath.Join(srcDir, "oc")
		if err := os.WriteFile(src, []byte("bin"), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"../oc", "sub/oc", "..", "/abs/oc"} {
			if err := atomicInstallFile(t.Context(), src, destDir, name); err == nil {
				t.Errorf("name %q must be rejected", name)
			}
		}
	})
}

func TestInstallToolsToSystem_AtomicAndExecutable(t *testing.T) {
	srcDir := t.TempDir()
	for _, name := range []string{"openshift-install", "oc"} {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(name+"-bin"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	p := newTestPhase(t)
	p.BinDir = t.TempDir()

	if err := p.InstallToolsToSystem(t.Context(), srcDir); err != nil {
		t.Fatalf("InstallToolsToSystem: %v", err)
	}

	for _, name := range []string{"openshift-install", "oc"} {
		info, err := os.Stat(filepath.Join(p.BinDir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if perm := info.Mode().Perm(); perm != 0o755 {
			t.Errorf("%s perm = %04o, want 0755", name, perm)
		}
	}

	// kubectl was absent from srcDir and must be skipped, not error.
	if _, err := os.Stat(filepath.Join(p.BinDir, "kubectl")); err == nil {
		t.Error("kubectl must not be installed when absent from srcDir")
	}
}
