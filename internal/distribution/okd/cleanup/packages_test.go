package cleanup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

func installFakePkgTools(t *testing.T) {
	t.Helper()
	for _, name := range []string{"rpm", "dnf", "dpkg", "apt-get"} {
		testutil.InstallFakeBin(t, name, "#!/bin/sh\nexit 0\n")
	}
}

func TestPackages_RemovesScopedBinariesOnly(t *testing.T) {
	installFakePkgTools(t)
	binDir := t.TempDir()

	for _, name := range InstalledBinaries() {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte("bin"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	unrelated := filepath.Join(binDir, "unrelated-tool")
	if err := os.WriteFile(unrelated, []byte("keep"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Packages(context.Background(), binDir, logutil.NopLogger); err != nil {
		t.Fatalf("Packages: %v", err)
	}

	for _, name := range InstalledBinaries() {
		if _, err := os.Stat(filepath.Join(binDir, name)); !os.IsNotExist(err) {
			t.Errorf("binary %q still present after cleanup", name)
		}
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Errorf("unrelated-tool was removed (should survive): %v", err)
	}
}

func TestPackages_RefusesCriticalBinDir(t *testing.T) {
	installFakePkgTools(t)
	for _, dir := range []string{"/", "/usr/local"} {
		err := Packages(context.Background(), dir, logutil.NopLogger)
		if err == nil {
			t.Errorf("Packages(binDir=%q) returned nil; want rejection", dir)
			continue
		}
		var clusterErr *errtypes.ClusterError
		if !errors.As(err, &clusterErr) {
			t.Errorf("Packages(binDir=%q) returned %T; want *errtypes.ClusterError", dir, err)
		}
	}
}

func TestPackages_MissingBinariesNoError(t *testing.T) {
	installFakePkgTools(t)
	binDir := t.TempDir()
	if err := Packages(context.Background(), binDir, logutil.NopLogger); err != nil {
		t.Fatalf("Packages with empty binDir returned error; want nil: %v", err)
	}
}
