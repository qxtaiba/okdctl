package provision

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

// goosLinux dedupes the "linux" literal so goconst doesn't flag 3+ occurrences.
const goosLinux = "linux"

func apacheCfg(webRoot string) *config.Config {
	cfg := config.DefaultConfig()
	cfg.HTTPServer.Root = webRoot
	return cfg
}

func writeIgnitionFixture(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(`{"ignition":{"version":"3.4.0"}}`), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func mustDeployToWebServer(t *testing.T, clusterDir, webRoot string) {
	t.Helper()
	p := newTestPhase(t)
	if err := p.DeployToWebServer(t.Context(), apacheCfg(webRoot), clusterDir); err != nil {
		t.Fatalf("DeployToWebServer: %v", err)
	}
}

func assertFileContainsAll(t *testing.T, path, label string, wants ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", label, err)
	}
	for _, want := range wants {
		if !strings.Contains(string(data), want) {
			t.Errorf("%s missing %q; got:\n%s", label, want, data)
		}
	}
}

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

func TestTeardownIgnitionServer_StopsUnconditionallyAndVerifies(t *testing.T) {
	if runtime.GOOS != goosLinux {
		t.Skip("systemctl branches are linux-only; darwin takes the GOOS gate")
	}
	// stop/disable succeed; the post-stop is-active verify reports inactive.
	callLog := fakeSystemctl(t, "case \"$1\" in is-active) exit 1;; *) exit 0;; esac")

	p := newTestPhase(t)
	if err := p.TeardownIgnitionServer(context.Background(), apacheCfg(t.TempDir())); err != nil {
		t.Fatalf("clean teardown must return nil: %v", err)
	}

	assertFileContainsAll(t, callLog, "systemctl calls",
		"stop httpd", "disable httpd", "is-active --quiet httpd")
}

func TestTeardownIgnitionServer_ReportsStillActive(t *testing.T) {
	if runtime.GOOS != goosLinux {
		t.Skip("systemctl branches are linux-only; darwin takes the GOOS gate")
	}
	// Every systemctl call "succeeds", including the post-stop is-active
	// check, simulating httpd surviving the stop.
	fakeSystemctl(t, "exit 0")

	p := newTestPhase(t)
	err := p.TeardownIgnitionServer(context.Background(), apacheCfg(t.TempDir()))
	if err == nil {
		t.Fatal("teardown must return an error when httpd is still active after the stop")
	}
	if !strings.Contains(err.Error(), "still active") {
		t.Errorf("error must name the still-active service: %v", err)
	}
}

func seedPublishedIgnition(t *testing.T, webRoot string, names ...string) {
	t.Helper()
	dir := filepath.Join(webRoot, "ignition")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeIgnitionFixture(t, dir, names...)
}

func TestTeardownIgnitionServer_UnpublishesWorkerIgnitionEvenWhenStopFails(t *testing.T) {
	fakeSystemctl(t, "exit 1")
	webRoot := t.TempDir()
	seedPublishedIgnition(t, webRoot, "worker.ign")

	p := newTestPhase(t)
	if err := p.TeardownIgnitionServer(t.Context(), apacheCfg(webRoot)); err == nil {
		t.Fatal("teardown must report that httpd could not be stopped")
	}
	if got := publishedIgnitionFiles(t, webRoot); len(got) != 0 {
		t.Errorf("published ignition files after teardown = %v; want none", got)
	}
}

func TestTeardownIgnitionServer_CleanStopUnpublishesWorkerIgnition(t *testing.T) {
	if runtime.GOOS != goosLinux {
		t.Skip("systemctl branches are linux-only; darwin takes the GOOS gate")
	}
	fakeSystemctl(t, "case \"$1\" in is-active) exit 1;; *) exit 0;; esac")
	webRoot := t.TempDir()
	seedPublishedIgnition(t, webRoot, "worker.ign")

	p := newTestPhase(t)
	if err := p.TeardownIgnitionServer(t.Context(), apacheCfg(webRoot)); err != nil {
		t.Fatalf("clean teardown must return nil: %v", err)
	}
	if got := publishedIgnitionFiles(t, webRoot); len(got) != 0 {
		t.Errorf("published ignition files after teardown = %v; want none", got)
	}
}

func publishedIgnitionFiles(t *testing.T, webRoot string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(webRoot, "ignition"))
	if err != nil {
		t.Fatalf("read published ignition dir: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestPublishWorkerIgnition_LeavesBootstrapAndMasterOutOfTheWebRoot(t *testing.T) {
	clusterDir, webRoot := t.TempDir(), t.TempDir()
	writeIgnitionFixture(t, clusterDir, IgnitionFilenames...)

	p := newTestPhase(t)
	if err := p.publishWorkerIgnition(t.Context(), apacheCfg(webRoot), clusterDir); err != nil {
		t.Fatalf("publishWorkerIgnition: %v", err)
	}
	if got, want := publishedIgnitionFiles(t, webRoot), []string{"worker.ign"}; !slices.Equal(got, want) {
		t.Errorf("published ignition files = %v; want %v", got, want)
	}
}

func TestReviveIgnitionServer_PublishesOnlyWorkerIgnition(t *testing.T) {
	if runtime.GOOS != goosLinux {
		t.Skip("systemctl branches are linux-only; darwin takes the GOOS gate")
	}
	redirectVhostDir(t)
	fakeSystemctl(t, "exit 0")

	clusterDir, webRoot := t.TempDir(), t.TempDir()
	writeIgnitionFixture(t, clusterDir, IgnitionFilenames...)
	cfg := apacheCfg(webRoot)
	cfg.HTTPServer.IgnitionServerIP = "127.0.0.1"

	p := newTestPhase(t)
	if err := p.ReviveIgnitionServer(t.Context(), cfg, "/root/okd", clusterDir); err != nil {
		t.Fatalf("ReviveIgnitionServer: %v", err)
	}
	if got, want := publishedIgnitionFiles(t, webRoot), []string{"worker.ign"}; !slices.Equal(got, want) {
		t.Errorf("published ignition files = %v; want %v", got, want)
	}
}

func TestDeployToWebServer_IgnitionFilesLandAt0640(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}

	clusterDir := t.TempDir()
	webRoot := t.TempDir()
	writeIgnitionFixture(t, clusterDir, IgnitionFilenames...)
	mustDeployToWebServer(t, clusterDir, webRoot)

	ignitionDir := filepath.Join(webRoot, "ignition")
	di, err := os.Stat(ignitionDir)
	if err != nil {
		t.Fatalf("stat ignition dir: %v", err)
	}
	if got := di.Mode().Perm(); got != 0o750 {
		t.Errorf("ignition dir perm = %04o, want 0750", got)
	}

	for _, name := range IgnitionFilenames {
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

	present := IgnitionFilenames[0]
	writeIgnitionFixture(t, clusterDir, present)
	mustDeployToWebServer(t, clusterDir, webRoot)

	ignitionDir := filepath.Join(webRoot, "ignition")
	if _, err := os.Stat(filepath.Join(ignitionDir, present)); err != nil {
		t.Errorf("%s missing in web root: %v", present, err)
	}

	for _, name := range IgnitionFilenames[1:] {
		if _, err := os.Stat(filepath.Join(ignitionDir, name)); err == nil {
			t.Errorf("%s must not exist in web root when absent from clusterDir", name)
		}
	}
}

func TestDeployToWebServer_AuthFilesNotCopied(t *testing.T) {
	clusterDir := t.TempDir()
	webRoot := t.TempDir()

	writeIgnitionFixture(t, clusterDir, IgnitionFilenames...)

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

	mustDeployToWebServer(t, clusterDir, webRoot)

	ignitionDir := filepath.Join(webRoot, "ignition")
	for _, rel := range authFiles {
		candidate := filepath.Join(ignitionDir, filepath.Base(rel))
		if _, err := os.Stat(candidate); err == nil {
			t.Errorf("auth file %s must not appear under ignition web root", rel)
		}
	}
}

func redirectVhostDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := apacheVhostConfDir
	apacheVhostConfDir = dir
	t.Cleanup(func() { apacheVhostConfDir = orig })
	return dir
}

func TestConfigureApacheHTTPS_RendersVhost(t *testing.T) {
	wantVhost := func(listen string) string {
		return "<VirtualHost " + listen + `:443>
  SSLEngine on
  SSLCertificateFile    /root/okd/certs/ignition/server.crt
  SSLCertificateKeyFile /root/okd/certs/ignition/server.key
  DocumentRoot /var/www/html
  <Directory "/var/www/html/ignition">
    Options None
    AllowOverride None
    Require all granted
  </Directory>
</VirtualHost>
`
	}
	cases := []struct {
		name   string
		bindIP string
		want   string
	}{
		{"no bind IP listens on all interfaces", "", wantVhost("*")},
		{"bind IP scopes the listen address", "192.168.1.20", wantVhost("192.168.1.20")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vhostDir := redirectVhostDir(t)
			certPath, keyPath := IgnitionCertPaths("/root/okd")

			p := newTestPhase(t)
			if err := p.configureApacheHTTPS(certPath, keyPath, "/var/www/html", tc.bindIP); err != nil {
				t.Fatalf("configureApacheHTTPS: %v", err)
			}

			got, err := os.ReadFile(filepath.Join(vhostDir, "ignition-ssl.conf"))
			if err != nil {
				t.Fatalf("vhost conf not written: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("vhost conf:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestConfigureApache_WiresVhostServiceAndIgnitionDir(t *testing.T) {
	if runtime.GOOS != goosLinux {
		t.Skip("systemctl branches are linux-only; darwin takes the GOOS gate")
	}
	vhostDir := redirectVhostDir(t)
	callLog := fakeSystemctl(t, "exit 0")

	webRoot := t.TempDir()
	cfg := apacheCfg(webRoot)
	cfg.HTTPServer.IgnitionServerIP = "127.0.0.1"

	p := newTestPhase(t)
	if err := p.ConfigureApache(t.Context(), cfg, "/root/okd"); err != nil {
		t.Fatalf("ConfigureApache: %v", err)
	}

	assertFileContainsAll(t, filepath.Join(vhostDir, "ignition-ssl.conf"), "vhost conf",
		"<VirtualHost 127.0.0.1:443>",
		"SSLCertificateFile    /root/okd/certs/ignition/server.crt",
		"DocumentRoot "+webRoot,
	)
	assertFileContainsAll(t, callLog, "systemctl calls", "enable httpd", "start httpd")

	fi, err := os.Stat(filepath.Join(webRoot, "ignition"))
	if err != nil {
		t.Fatalf("ignition dir not created: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o750 {
		t.Errorf("ignition dir perm = %04o, want 0750", got)
	}
}
