package provision

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/platform"
	"github.com/qxtaiba/okdctl/internal/system"
)

const minIgnitionFileSize = 1000 // bytes

// apacheVhostConfDirFn resolves the vhost drop-in dir; tests override it to
// redirect writes to a t.TempDir().
var apacheVhostConfDirFn = platform.OS.ApacheVhostConfDir

func (p *Provisioner) ensureIgnitionDir(ctx context.Context, webRoot string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	ignitionDir := filepath.Join(webRoot, "ignition")

	// 0o750: apache owns/reads; other local users can't read ignition files,
	// which embed the pull-secret.
	if err := os.MkdirAll(ignitionDir, 0o750); err != nil {
		return "", &errtypes.ConfigError{Msg: "create ignition directory", Err: err}
	}

	// Runs as root over a path a non-root user can influence; MkdirAll,
	// ChownByName, and Chmod all follow symlinks, so refuse one first,
	// matching system/fs.go.
	if info, err := os.Lstat(ignitionDir); err != nil {
		return "", &errtypes.AuthError{Msg: fmt.Sprintf("lstat ignition dir %q", ignitionDir), Err: err}
	} else if info.Mode()&os.ModeSymlink != 0 {
		return "", &errtypes.AuthError{Msg: fmt.Sprintf("ignition dir %q is a symlink; refusing to chown/chmod", ignitionDir), Err: os.ErrPermission}
	}

	apacheUser := p.OS.ApacheUser()
	if err := system.ChownByName(ignitionDir, apacheUser+":"+apacheUser); err != nil {
		p.Log.Warn("apache: failed to set ignition dir ownership", "err", err)
	}
	if err := os.Chmod(ignitionDir, 0o750); err != nil {
		p.Log.Warn("apache: failed to set ignition dir permissions", "err", err)
	}

	return ignitionDir, nil
}

func enableAndStartApache(ctx context.Context, serviceName string) error {
	if err := system.ManageService(ctx, system.ServiceEnable, serviceName); err != nil {
		return fmt.Errorf("enable %s: %w", serviceName, err)
	}
	return startApache(ctx, serviceName)
}

func startApache(ctx context.Context, serviceName string) error {
	if err := system.ManageService(ctx, system.ServiceStart, serviceName); err != nil {
		return fmt.Errorf("start %s: %w", serviceName, err)
	}
	if !system.IsServiceActive(ctx, serviceName) {
		return fmt.Errorf("apache service %s failed to start - check systemctl status %s", serviceName, serviceName)
	}
	return nil
}

func (p *Provisioner) verifyApacheListening(ctx context.Context, bindIP string) {
	addr := "127.0.0.1:443"
	if bindIP != "" {
		addr = net.JoinHostPort(bindIP, "443")
	}
	dialer := &net.Dialer{Timeout: 1 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		p.Log.Warn("apache: httpd may not be listening on port 443 - check configuration", "addr", addr, "err", err)
		return
	}
	_ = conn.Close()
	p.Log.Info("apache: httpd service listening on port 443")
}

// configureApacheHTTPS writes the HTTPS vhost conf and, on Debian, enables
// mod_ssl and the conf (RHEL auto-includes conf.d).
func (p *Provisioner) configureApacheHTTPS(ctx context.Context, certPath, keyPath, webRoot, bindIP string) error {
	vhostDir := apacheVhostConfDirFn(p.OS)
	if err := system.EnsureDir(vhostDir); err != nil {
		return fmt.Errorf("apache: ensure vhost conf dir: %w", err)
	}

	listen := "*:443"
	if bindIP != "" {
		listen = net.JoinHostPort(bindIP, "443")
	}

	vhostConf := fmt.Sprintf("<VirtualHost %s>\n  SSLEngine on\n  SSLCertificateFile    %s\n  SSLCertificateKeyFile %s\n  DocumentRoot %s\n  <Directory \"%s/ignition\">\n    Options None\n    AllowOverride None\n    Require all granted\n  </Directory>\n</VirtualHost>\n",
		listen, certPath, keyPath, webRoot, webRoot)

	confPath := filepath.Join(vhostDir, "ignition-ssl.conf")
	if err := system.AtomicWriteString(confPath, vhostConf, 0o644); err != nil {
		return fmt.Errorf("apache: write HTTPS vhost conf: %w", err)
	}

	if p.OS.Family == platform.FamilyDebian {
		if _, err := p.Exec.RunChecked(ctx, "a2enmod", "ssl"); err != nil {
			p.Log.Warn("apache: a2enmod ssl failed", "err", err)
		}
		if _, err := p.Exec.RunChecked(ctx, "a2enconf", "ignition-ssl"); err != nil {
			p.Log.Warn("apache: a2enconf ignition-ssl failed", "err", err)
		}
	}

	p.Log.Info("apache: HTTPS vhost configured", "path", confPath)
	return nil
}

// ConfigureApache configures httpd to serve ignition payloads over HTTPS:
// writes the TLS vhost conf, enables the service, and creates the ignition
// directory. TLS with a pinned CA cert is the primary defence against
// credential capture on the machine-network VLAN.
func (p *Provisioner) ConfigureApache(ctx context.Context, cfg *config.Config, projectRoot string) error {
	p.Log.Info("apache: configuring httpd for serving ignition files over https")

	bindIP := cfg.HTTPServer.IgnitionServerIP
	// 443 is already labeled https_port_t under RHEL SELinux, so no port
	// relabel is needed here.

	webRoot := cfg.HTTPServer.Root
	if webRoot == "" {
		webRoot = phase.DefaultHTTPServerRoot
	}

	certPath, keyPath := IgnitionCertPaths(projectRoot)
	if err := p.configureApacheHTTPS(ctx, certPath, keyPath, webRoot, bindIP); err != nil {
		return &errtypes.ClusterError{Msg: "configure apache HTTPS vhost", Err: err}
	}

	if err := enableAndStartApache(ctx, p.OS.ApacheServiceName()); err != nil {
		return &errtypes.ClusterError{Msg: "enable and start apache", Err: err}
	}

	p.verifyApacheListening(ctx, bindIP)

	ignitionDir, err := p.ensureIgnitionDir(ctx, webRoot)
	if err != nil {
		return err
	}

	p.Log.Info("apache: ignition directory created", "path", ignitionDir)
	return nil
}

// ReviveIgnitionServer reopens the ignition join window for node add: the
// same vhost/dir setup as ConfigureApache, plus a re-deploy of ignition
// payloads into the web root. The service is started WITHOUT being
// enabled, so a crash mid-add can't resurrect the pull-secret server
// across reboots.
func (p *Provisioner) ReviveIgnitionServer(ctx context.Context, cfg *config.Config, projectRoot, clusterDir string) error {
	p.Log.Info("apache: reviving httpd for the node-add join window")

	bindIP := cfg.HTTPServer.IgnitionServerIP
	webRoot := cfg.HTTPServer.Root
	if webRoot == "" {
		webRoot = phase.DefaultHTTPServerRoot
	}

	certPath, keyPath := IgnitionCertPaths(projectRoot)
	if err := p.configureApacheHTTPS(ctx, certPath, keyPath, webRoot, bindIP); err != nil {
		return &errtypes.ClusterError{Msg: "configure apache https vhost", Err: err}
	}
	if err := startApache(ctx, p.OS.ApacheServiceName()); err != nil {
		return &errtypes.ClusterError{Msg: "start apache", Err: err}
	}
	p.verifyApacheListening(ctx, bindIP)

	return p.DeployToWebServer(ctx, cfg, clusterDir)
}

// TeardownIgnitionServer stops and disables httpd once a node-add join
// window closes, verifying the stop took; the stop runs unconditionally
// (not gated on an is-active probe) since teardown runs under a detached
// post-cancel context. A non-nil return means httpd may still be serving
// ignition payloads — the caller must surface that loudly.
func (p *Provisioner) TeardownIgnitionServer(ctx context.Context) error {
	svc := p.OS.ApacheServiceName()
	var errs []error
	if err := system.ManageService(ctx, system.ServiceStop, svc); err != nil {
		errs = append(errs, fmt.Errorf("stop %s: %w", svc, err))
	}
	if err := system.ManageService(ctx, system.ServiceDisable, svc); err != nil {
		p.Log.Warn("apache: disable after ignition teardown failed", "svc", svc, "err", err)
	}
	if system.IsServiceActive(ctx, svc) {
		errs = append(errs, fmt.Errorf("%s still active after stop", svc))
	}
	return errors.Join(errs...)
}

// DeployToWebServer copies the generated ignition files from clusterDir into
// the httpd web root. Auth credentials (kubeconfig, kubeadmin-password) are
// intentionally not copied — they must never be placed under the apache
// DocumentRoot.
func (p *Provisioner) DeployToWebServer(ctx context.Context, cfg *config.Config, clusterDir string) error {
	webRoot := cfg.HTTPServer.Root
	if webRoot == "" {
		webRoot = phase.DefaultHTTPServerRoot
	}

	ignitionDir, err := p.ensureIgnitionDir(ctx, webRoot)
	if err != nil {
		return err
	}

	for _, file := range IgnitionFilenames {
		srcPath := filepath.Join(clusterDir, file)
		if !system.FileExists(srcPath) {
			continue
		}

		data, err := os.ReadFile(srcPath)
		if err != nil {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("read %s", file), Err: err}
		}

		destPath := filepath.Join(ignitionDir, file)
		// 0o640: apache-group-readable only (payload carries pullSecret);
		// AtomicWrite so a booting node never fetches a torn copy.
		if err := system.AtomicWrite(destPath, data, 0o640); err != nil {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("copy %s", file), Err: err}
		}
	}

	return nil
}

// IgnitionDeployAlreadyDone reports whether every generated ignition file in
// clusterDir has a byte-identical copy under webRoot/ignition — mere
// existence isn't enough since a regenerated ignition embeds a fresh
// cluster CA. Any read failure conservatively returns false so Exec
// re-deploys.
func IgnitionDeployAlreadyDone(clusterDir, webRoot string) bool {
	ignitionDir := filepath.Join(webRoot, "ignition")
	for _, name := range IgnitionFilenames {
		srcSum, err := fileSHA256(filepath.Join(clusterDir, name))
		if err != nil {
			return false
		}
		deployedSum, err := fileSHA256(filepath.Join(ignitionDir, name))
		if err != nil || deployedSum != srcSum {
			return false
		}
	}
	return true
}

// fileSHA256 streams path through sha256 instead of reading it whole, so the
// pull-secret-bearing payload is never fully buffered.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyWebServer fetches bootstrap.ign over HTTPS from baseURL and verifies
// the server certificate against caCertPEM. A mismatch fails the TLS
// handshake, confirming Apache serves the cert embedded into node ISOs via
// --ignition-ca.
func (p *Provisioner) VerifyWebServer(ctx context.Context, baseURL string, caCertPEM []byte) error {
	testURL := fmt.Sprintf("%s/bootstrap.ign", baseURL)

	block, _ := pem.Decode(caCertPEM)
	if block == nil {
		return &errtypes.ConfigError{Msg: "ignition ca pem is not a valid PEM block"}
	}
	cert, parseErr := x509.ParseCertificate(block.Bytes)
	if parseErr != nil {
		return &errtypes.ConfigError{Msg: "parse ignition ca cert", Err: parseErr}
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	client := httputil.NewWithCA(pool, httputil.TimeoutShort)

	p.Log.Info("apache: verifying https web server", "url", testURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, http.NoBody)
	if err != nil {
		return &errtypes.NetworkError{Msg: "create request", Err: err}
	}

	resp, err := client.Do(req)
	if err != nil {
		return &errtypes.NetworkError{Msg: "connect to web server", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return &errtypes.NetworkError{Msg: fmt.Sprintf("web server returned status %d for %s", resp.StatusCode, testURL)}
	}

	if resp.ContentLength > 0 && resp.ContentLength < minIgnitionFileSize {
		return &errtypes.NetworkError{Msg: fmt.Sprintf("bootstrap.ign appears too small (%d bytes)", resp.ContentLength)}
	}

	p.Log.Info("apache: https web server accessible and serving ignition files")

	return nil
}
