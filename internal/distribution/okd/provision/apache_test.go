package provision

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/platform"
	"github.com/qxtaiba/okdctl/internal/testutil"
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

func TestTeardownIgnitionServer_StopsUnconditionallyAndVerifies(t *testing.T) {
	if runtime.GOOS != goosLinux {
		t.Skip("systemctl branches are linux-only; darwin takes the GOOS gate")
	}
	// stop/disable succeed; the post-stop is-active verify reports inactive.
	callLog := fakeSystemctl(t, "case \"$1\" in is-active) exit 1;; *) exit 0;; esac")

	p := newTestPhase(t)
	if err := p.TeardownIgnitionServer(context.Background()); err != nil {
		t.Fatalf("clean teardown must return nil: %v", err)
	}

	calls, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	// The stop is NOT gated on an is-active probe: teardown runs under a
	// detached post-cancel ctx where a failing probe would silently skip the
	// stop and leave the pull-secret window open.
	for _, want := range []string{"stop httpd", "disable httpd", "is-active --quiet httpd"} {
		if !strings.Contains(string(calls), want) {
			t.Errorf("systemctl calls missing %q; got:\n%s", want, calls)
		}
	}
}

func TestTeardownIgnitionServer_ReportsStillActive(t *testing.T) {
	if runtime.GOOS != goosLinux {
		t.Skip("systemctl branches are linux-only; darwin takes the GOOS gate")
	}
	// Every call "succeeds", including the post-stop is-active verify — the
	// service survived the stop, so teardown must fail loudly rather than let
	// node add report success while the pull secret is still served.
	fakeSystemctl(t, "exit 0")

	p := newTestPhase(t)
	err := p.TeardownIgnitionServer(context.Background())
	if err == nil {
		t.Fatal("teardown must return an error when httpd is still active after the stop")
	}
	if !strings.Contains(err.Error(), "still active") {
		t.Errorf("error must name the still-active service: %v", err)
	}
}

func TestDeployToWebServer_IgnitionFilesLandAt0640(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}

	clusterDir := t.TempDir()
	webRoot := t.TempDir()

	for _, name := range IgnitionFilenames {
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

	for _, name := range IgnitionFilenames[1:] {
		if _, err := os.Stat(filepath.Join(ignitionDir, name)); err == nil {
			t.Errorf("%s must not exist in web root when absent from clusterDir", name)
		}
	}
}

func TestDeployToWebServer_AuthFilesNotCopied(t *testing.T) {
	clusterDir := t.TempDir()
	webRoot := t.TempDir()

	for _, name := range IgnitionFilenames {
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

func redirectVhostDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := apacheVhostConfDirFn
	apacheVhostConfDirFn = func(platform.OS) string { return dir }
	t.Cleanup(func() { apacheVhostConfDirFn = orig })
	return dir
}

func TestConfigureApacheHTTPS_RendersVhost(t *testing.T) {
	cases := []struct {
		name   string
		bindIP string
		want   string
	}{
		{
			name:   "no bind IP listens on all interfaces",
			bindIP: "",
			want: `<VirtualHost 443>
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
`,
		},
		{
			name:   "bind IP scopes the listen address",
			bindIP: "192.168.1.20",
			want: `<VirtualHost 192.168.1.20:443>
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
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vhostDir := redirectVhostDir(t)
			certPath, keyPath := IgnitionCertPaths("/root/okd")

			p := newTestPhase(t)
			if err := p.configureApacheHTTPS(t.Context(), certPath, keyPath, "/var/www/html", tc.bindIP); err != nil {
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

func TestConfigureApacheHTTPS_DebianEnablesModAndConf(t *testing.T) {
	redirectVhostDir(t)
	logPath := filepath.Join(t.TempDir(), "a2.log")
	script := "#!/bin/sh\necho \"$(basename \"$0\") $@\" >> " + logPath + "\nexit 0\n"
	testutil.InstallFakeBin(t, "a2enmod", script)
	testutil.InstallFakeBin(t, "a2enconf", script)

	p := newTestPhase(t)
	p.OS = platform.OS{Family: platform.FamilyDebian}
	if err := p.configureApacheHTTPS(t.Context(), "/c.crt", "/c.key", "/var/www/html", ""); err != nil {
		t.Fatalf("configureApacheHTTPS: %v", err)
	}

	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("a2 tools not invoked: %v", err)
	}
	for _, want := range []string{"a2enmod ssl", "a2enconf ignition-ssl"} {
		if !strings.Contains(string(calls), want) {
			t.Errorf("a2 calls missing %q; got:\n%s", want, calls)
		}
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

	conf, err := os.ReadFile(filepath.Join(vhostDir, "ignition-ssl.conf"))
	if err != nil {
		t.Fatalf("vhost conf not written: %v", err)
	}
	for _, want := range []string{
		"<VirtualHost 127.0.0.1:443>",
		"SSLCertificateFile    /root/okd/certs/ignition/server.crt",
		"DocumentRoot " + webRoot,
	} {
		if !strings.Contains(string(conf), want) {
			t.Errorf("vhost conf missing %q; got:\n%s", want, conf)
		}
	}

	calls, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatalf("read systemctl log: %v", err)
	}
	for _, want := range []string{"enable httpd", "start httpd"} {
		if !strings.Contains(string(calls), want) {
			t.Errorf("systemctl calls missing %q; got:\n%s", want, calls)
		}
	}

	fi, err := os.Stat(filepath.Join(webRoot, "ignition"))
	if err != nil {
		t.Fatalf("ignition dir not created: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o750 {
		t.Errorf("ignition dir perm = %04o, want 0750", got)
	}
}
