package platform

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// installFakePkgTools writes controllable fake binaries into a TempDir and
// prepends it to PATH for the duration of the test. It returns the path of
// an argv log that dnf and apt-get append each invocation to (one line of
// space-joined args per call), so tests can assert what the package tool
// was — or was not — invoked with. The path is embedded in the scripts
// because system.RunCaptured strips non-allowlisted env vars.
//
// Behaviour:
//   rpm      exits 1 when its last argument is "notinstalled", else 0.
//   dpkg     exits 0; stdout is "ii  <arg>"; if arg == "rcpkg" stdout is
//            "rc  rcpkg" (simulating a stale removed entry).
//   dnf      logs argv, always exits 0.
//   apt-get  logs argv, always exits 0.
//
// eval "last=\$$#" retrieves the last positional argument portably under
// POSIX sh (dash on Debian/Ubuntu CI), avoiding the bash-only ${@: -1}.
func installFakePkgTools(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake pkg-tool scripts require POSIX sh")
	}
	dir := t.TempDir()
	argvLog := filepath.Join(dir, "argv.log")
	logArgv := "#!/bin/sh\necho \"$*\" >> \"" + argvLog + "\"\nexit 0\n"
	scripts := map[string]string{
		"rpm":     "#!/bin/sh\neval \"last=\\$$#\"\nif [ \"$last\" = \"notinstalled\" ]; then exit 1; fi\nexit 0\n",
		"dpkg":    "#!/bin/sh\neval \"last=\\$$#\"\nif [ \"$last\" = \"rcpkg\" ]; then\n  printf 'rc  rcpkg\\n'\nelse\n  printf 'ii  %s\\n' \"$last\"\nfi\nexit 0\n",
		"dnf":     logArgv,
		"apt-get": logArgv,
	}
	for name, body := range scripts {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argvLog
}

// readArgvLog returns the argv log contents, or "" when the package tool
// was never invoked (log file absent).
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

// redirectAptListDir points aptListDir at a TempDir for the duration of
// the test and restores the original value via t.Cleanup.
func redirectAptListDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := aptListDir
	aptListDir = dir
	t.Cleanup(func() { aptListDir = orig })
	return dir
}

func TestIsInstalled_RHEL(t *testing.T) {
	installFakePkgTools(t)
	m := NewPackageManager(OS{Family: FamilyRHEL})

	tests := []struct {
		pkg    string
		wantOK bool
	}{
		{"installed-pkg", true},
		{"notinstalled", false},
	}
	for _, tc := range tests {
		ok, err := m.IsInstalled(context.Background(), tc.pkg)
		if err != nil {
			t.Errorf("IsInstalled(%q): unexpected error: %v", tc.pkg, err)
			continue
		}
		if ok != tc.wantOK {
			t.Errorf("IsInstalled(%q) = %v; want %v", tc.pkg, ok, tc.wantOK)
		}
	}
}

func TestIsInstalled_Debian(t *testing.T) {
	installFakePkgTools(t)
	m := NewPackageManager(OS{Family: FamilyDebian})

	tests := []struct {
		pkg    string
		wantOK bool
	}{
		{"present-pkg", true},
		{"rcpkg", false},
	}
	for _, tc := range tests {
		ok, err := m.IsInstalled(context.Background(), tc.pkg)
		if err != nil {
			t.Errorf("IsInstalled(%q): unexpected error: %v", tc.pkg, err)
			continue
		}
		if ok != tc.wantOK {
			t.Errorf("IsInstalled(%q) = %v; want %v", tc.pkg, ok, tc.wantOK)
		}
	}
}

func TestIsInstalled_LookPathError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX PATH semantics")
	}
	t.Setenv("PATH", "")
	m := NewPackageManager(OS{Family: FamilyRHEL})
	_, err := m.IsInstalled(context.Background(), "any")
	if err == nil {
		t.Error("IsInstalled with empty PATH: want error, got nil")
	}
}

func TestRemove_EmptyInput(t *testing.T) {
	argvLog := installFakePkgTools(t)
	m := NewPackageManager(OS{Family: FamilyRHEL})
	if err := m.Remove(context.Background(), nil, nil); err != nil {
		t.Fatalf("Remove(nil): unexpected error: %v", err)
	}
	if got := readArgvLog(t, argvLog); got != "" {
		t.Errorf("Remove(nil) invoked dnf: %q; want no invocation", got)
	}
}

func TestRemove_AllUninstalled(t *testing.T) {
	argvLog := installFakePkgTools(t)
	m := NewPackageManager(OS{Family: FamilyRHEL})
	if err := m.Remove(context.Background(), []string{"notinstalled"}, nil); err != nil {
		t.Fatalf("Remove(all-uninstalled): unexpected error: %v", err)
	}
	if got := readArgvLog(t, argvLog); got != "" {
		t.Errorf("Remove(all-uninstalled) invoked dnf: %q; want no invocation", got)
	}
}

func TestRemove_InstalledPackage(t *testing.T) {
	argvLog := installFakePkgTools(t)
	m := NewPackageManager(OS{Family: FamilyRHEL})
	if err := m.Remove(context.Background(), []string{"installed-pkg"}, nil); err != nil {
		t.Fatalf("Remove(installed): unexpected error: %v", err)
	}
	want := "remove -y installed-pkg\n"
	if got := readArgvLog(t, argvLog); got != want {
		t.Errorf("dnf argv log = %q; want %q", got, want)
	}
}

func TestRemove_MixedPackages(t *testing.T) {
	argvLog := installFakePkgTools(t)
	m := NewPackageManager(OS{Family: FamilyRHEL})
	if err := m.Remove(context.Background(), []string{"notinstalled", "installed-pkg"}, nil); err != nil {
		t.Fatalf("Remove(mixed): unexpected error: %v", err)
	}
	want := "remove -y installed-pkg\n"
	if got := readArgvLog(t, argvLog); got != want {
		t.Errorf("dnf argv log = %q; want %q", got, want)
	}
}

func TestAddRepo_RHEL(t *testing.T) {
	installFakePkgTools(t)
	m := NewPackageManager(OS{Family: FamilyRHEL})
	if err := m.AddRepo(context.Background(), "myrepo", "https://repo.example.com", logutil.NopLogger); err != nil {
		t.Fatalf("AddRepo RHEL: unexpected error: %v", err)
	}
}

func TestAddRepo_Debian_Content(t *testing.T) {
	installFakePkgTools(t)
	listDir := redirectAptListDir(t)

	m := NewPackageManager(OS{Family: FamilyDebian})
	const (
		repoName = "myrepo"
		repoURL  = "https://repo.example.com"
	)
	if err := m.AddRepo(context.Background(), repoName, repoURL, logutil.NopLogger); err != nil {
		t.Fatalf("AddRepo Debian: unexpected error: %v", err)
	}

	listFile := filepath.Join(listDir, repoName+".list")
	got, err := os.ReadFile(listFile)
	if err != nil {
		t.Fatalf("list file not written: %v", err)
	}
	want := "deb [arch=" + DownloadArch() + "] " + repoURL + " any main\n"
	if string(got) != want {
		t.Errorf("list file content:\ngot  %q\nwant %q", string(got), want)
	}
}
