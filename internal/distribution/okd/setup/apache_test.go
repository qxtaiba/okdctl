package setup

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

// goosLinux dedupes the "linux" literal across the systemctl-gated tests
// below (goconst flags 3+ occurrences).
const goosLinux = "linux"

func apacheCfg(webRoot string) *config.Config {
	cfg := config.DefaultConfig()
	cfg.HTTPServer.Root = webRoot
	return cfg
}

// fakeSystemctl writes an executable systemctl stub that appends its argv to
// calls.log and prepends its dir to PATH, mirroring
// phase/teardown_test.go's fakeTeardownBin pattern.
func fakeSystemctl(t *testing.T, script string) (callLog string) {
	t.Helper()
	dir := t.TempDir()
	callLog = filepath.Join(dir, "calls.log")
	body := "#!/bin/sh\necho \"$@\" >> " + callLog + "\n" + script + "\n"
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return callLog
}

func TestTeardownIgnitionServer_StopsAndDisablesHTTPD(t *testing.T) {
	if runtime.GOOS != goosLinux {
		t.Skip("systemctl branches are linux-only; darwin takes the GOOS gate")
	}
	callLog := fakeSystemctl(t, "exit 0")

	p := newTestPhase(t)
	p.TeardownIgnitionServer(context.Background())

	calls, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	for _, want := range []string{"is-active --quiet httpd", "stop httpd", "is-enabled --quiet httpd", "disable httpd"} {
		if !strings.Contains(string(calls), want) {
			t.Errorf("systemctl calls missing %q; got:\n%s", want, calls)
		}
	}
}

func TestTeardownIgnitionServer_InactiveIsNoop(t *testing.T) {
	var callLog string
	if runtime.GOOS == goosLinux {
		callLog = fakeSystemctl(t, "case \"$1\" in is-*) exit 1;; *) exit 0;; esac")
	}

	p := newTestPhase(t)
	p.TeardownIgnitionServer(context.Background())

	if runtime.GOOS == goosLinux {
		calls, err := os.ReadFile(callLog)
		if err != nil {
			t.Fatalf("read call log: %v", err)
		}
		if strings.Contains(string(calls), "stop httpd") || strings.Contains(string(calls), "disable httpd") {
			t.Errorf("inactive httpd must not be stopped or disabled; got:\n%s", calls)
		}
	}
}

func TestDeployToWebServer_IgnitionFilesLandAt0640(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}

	clusterDir := t.TempDir()
	webRoot := t.TempDir()

	for _, name := range ignitionFilenames {
		path := filepath.Join(clusterDir, name)
		if err := os.WriteFile(path, []byte(`{"ignition":{"version":"3.4.0"}}`), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	p := newTestPhase(t)
	if err := p.DeployToWebServer(t.Context(), apacheCfg(webRoot), clusterDir); err != nil {
		t.Fatalf("DeployToWebServer: %v", err)
	}

	ignitionDir := filepath.Join(webRoot, "ignition")
	di, err := os.Stat(ignitionDir)
	if err != nil {
		t.Fatalf("stat ignition dir: %v", err)
	}
	if got := di.Mode().Perm(); got != 0o750 {
		t.Errorf("ignition dir perm = %04o, want 0750", got)
	}

	for _, name := range ignitionFilenames {
		fi, err := os.Stat(filepath.Join(ignitionDir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if got := fi.Mode().Perm(); got != 0o640 {
			t.Errorf("%s perm = %04o, want 0640", name, got)
		}
	}
}

func TestDeployToWebServer_AbsentFilesSkipped(t *testing.T) {
	clusterDir := t.TempDir()
	webRoot := t.TempDir()

	present := ignitionFilenames[0]
	if err := os.WriteFile(
		filepath.Join(clusterDir, present),
		[]byte(`{"ignition":{"version":"3.4.0"}}`),
		0o600,
	); err != nil {
		t.Fatalf("write %s: %v", present, err)
	}

	p := newTestPhase(t)
	if err := p.DeployToWebServer(t.Context(), apacheCfg(webRoot), clusterDir); err != nil {
		t.Fatalf("DeployToWebServer: %v", err)
	}

	ignitionDir := filepath.Join(webRoot, "ignition")
	if _, err := os.Stat(filepath.Join(ignitionDir, present)); err != nil {
		t.Errorf("%s missing in web root: %v", present, err)
	}

	for _, name := range ignitionFilenames[1:] {
		if _, err := os.Stat(filepath.Join(ignitionDir, name)); err == nil {
			t.Errorf("%s must not exist in web root when absent from clusterDir", name)
		}
	}
}

func TestDeployToWebServer_AuthFilesNotCopied(t *testing.T) {
	clusterDir := t.TempDir()
	webRoot := t.TempDir()

	for _, name := range ignitionFilenames {
		if err := os.WriteFile(
			filepath.Join(clusterDir, name),
			[]byte(`{"ignition":{"version":"3.4.0"}}`),
			0o600,
		); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	authFiles := []string{"auth/kubeconfig", "kubeadmin-password"}
	if err := os.MkdirAll(filepath.Join(clusterDir, "auth"), 0o700); err != nil {
		t.Fatalf("mkdir auth: %v", err)
	}
	for _, rel := range authFiles {
		if err := os.WriteFile(
			filepath.Join(clusterDir, rel),
			[]byte("fake-credential"),
			0o600,
		); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	p := newTestPhase(t)
	if err := p.DeployToWebServer(t.Context(), apacheCfg(webRoot), clusterDir); err != nil {
		t.Fatalf("DeployToWebServer: %v", err)
	}

	ignitionDir := filepath.Join(webRoot, "ignition")
	for _, rel := range authFiles {
		candidate := filepath.Join(ignitionDir, filepath.Base(rel))
		if _, err := os.Stat(candidate); err == nil {
			t.Errorf("auth file %s must not appear under ignition web root", rel)
		}
	}
}
