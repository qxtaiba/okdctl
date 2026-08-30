package platform

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

// installFakePkgTools installs fake dnf/apt-get/rpm/dpkg on PATH and returns
// the argv log path (env-embedded since RunCaptured strips non-allowlisted
// vars).
func installFakePkgTools(t *testing.T) string {
	t.Helper()
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	logArgv := "#!/bin/sh\necho \"$*\" >> \"" + argvLog + "\"\nexit 0\n"
	scripts := map[string]string{
		"rpm":     "#!/bin/sh\neval \"last=\\$$#\"\nif [ \"$last\" = \"notinstalled\" ]; then exit 1; fi\nexit 0\n",
		"dpkg":    "#!/bin/sh\neval \"last=\\$$#\"\nif [ \"$last\" = \"rcpkg\" ]; then\n  printf 'rc  rcpkg\\n'\nelse\n  printf 'ii  %s\\n' \"$last\"\nfi\nexit 0\n",
		"dnf":     logArgv,
		"apt-get": logArgv,
	}
	for name, body := range scripts {
		testutil.InstallFakeBin(t, name, body)
	}
	return argvLog
}

// readArgvLog returns the argv log contents, or "" if never invoked.
func readArgvLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	return string(data)
}

func TestIsInstalled(t *testing.T) {
	installFakePkgTools(t)

	tests := []struct {
		family Family
		pkg    string
		wantOK bool
	}{
		{FamilyRHEL, "installed-pkg", true},
		{FamilyRHEL, "notinstalled", false},
		{FamilyDebian, "present-pkg", true},
		{FamilyDebian, "rcpkg", false},
	}
	for _, tc := range tests {
		m := NewPackageManager(OS{Family: tc.family}, logutil.NopLogger)
		ok, err := m.isInstalled(context.Background(), tc.pkg)
		if err != nil {
			t.Errorf("%s isInstalled(%q): unexpected error: %v", tc.family, tc.pkg, err)
			continue
		}
		if ok != tc.wantOK {
			t.Errorf("%s isInstalled(%q) = %v; want %v", tc.family, tc.pkg, ok, tc.wantOK)
		}
	}
}

func TestIsInstalled_LookPathError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX PATH semantics")
	}
	t.Setenv("PATH", "")
	m := NewPackageManager(OS{Family: FamilyRHEL}, logutil.NopLogger)
	_, err := m.isInstalled(context.Background(), "any")
	if err == nil {
		t.Error("isInstalled with empty PATH: want error, got nil")
	}
}

func TestRemove(t *testing.T) {
	cases := []struct {
		name     string
		packages []string
		wantArgv string // "" means the package tool must never run
	}{
		{"empty input", nil, ""},
		{"all uninstalled", []string{"notinstalled"}, ""},
		{"installed package", []string{"installed-pkg"}, "remove -y installed-pkg\n"},
		{"mixed packages", []string{"notinstalled", "installed-pkg"}, "remove -y installed-pkg\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			argvLog := installFakePkgTools(t)
			m := NewPackageManager(OS{Family: FamilyRHEL}, logutil.NopLogger)
			if err := m.Remove(context.Background(), tc.packages); err != nil {
				t.Fatalf("Remove(%v): unexpected error: %v", tc.packages, err)
			}
			if got := readArgvLog(t, argvLog); got != tc.wantArgv {
				t.Errorf("dnf argv log = %q; want %q", got, tc.wantArgv)
			}
		})
	}
}
