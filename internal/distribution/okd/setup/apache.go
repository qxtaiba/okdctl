package setup

import (
	"context"
	"crypto/x509"
	"encoding/pem"
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

const (
	minIgnitionFileSize = 1000 // bytes
)

func (p *Phase) ensureIgnitionDir(ctx context.Context, webRoot string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	ignitionDir := filepath.Join(webRoot, "ignition")

	// 0o750: apache user owns and reads; local non-apache users cannot read
	// ignition files which embed the cluster pull-secret.
	if err := os.MkdirAll(ignitionDir, 0o750); err != nil {
		return "", &errtypes.ConfigError{Msg: "failed to create ignition directory", Err: err}
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
		return fmt.Errorf("failed to enable %s: %w", serviceName, err)
	}
	if err := system.ManageService(ctx, system.ServiceStart, serviceName); err != nil {
		return fmt.Errorf("failed to start %s: %w", serviceName, err)
	}
	if !system.IsServiceActive(ctx, serviceName) {
		return fmt.Errorf("apache service %s failed to start - check systemctl status %s", serviceName, serviceName)
	}
	return nil
}

func (p *Phase) verifyApacheListening(ctx context.Context, bindIP string) {
	addr := "127.0.0.1:443"
	if bindIP != "" {
		addr = net.JoinHostPort(bindIP, "443")
	}
	dialer := &net.Dialer{Timeout: 1 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		p.Log.Warn("apache: httpd may not be listening on port 443 - check configuration")
		return
	}
	_ = conn.Close()
	p.Log.Info("apache: httpd service listening on port 443")
}

// configureApacheHTTPS writes the HTTPS vhost drop-in conf and, on Debian,
// enables mod_ssl and the conf. On RHEL conf.d is auto-included by httpd.conf.
func (p *Phase) configureApacheHTTPS(ctx context.Context, certPath, keyPath, webRoot, bindIP string) error {
	vhostDir := p.OS.ApacheVhostConfDir()
	if err := system.EnsureDir(vhostDir); err != nil {
		return fmt.Errorf("apache: failed to ensure vhost conf dir: %w", err)
	}

	listen := "443"
	if bindIP != "" {
		listen = net.JoinHostPort(bindIP, "443")
	}

	vhostConf := fmt.Sprintf("<VirtualHost %s>\n  SSLEngine on\n  SSLCertificateFile    %s\n  SSLCertificateKeyFile %s\n  DocumentRoot %s\n  <Directory \"%s/ignition\">\n    Options None\n    AllowOverride None\n    Require all granted\n  </Directory>\n</VirtualHost>\n",
		listen, certPath, keyPath, webRoot, webRoot)

	confPath := filepath.Join(vhostDir, "ignition-ssl.conf")
	if err := system.AtomicWriteString(confPath, vhostConf, 0o644); err != nil {
		return fmt.Errorf("apache: failed to write HTTPS vhost conf: %w", err)
	}

	if p.OS.Family == platform.FamilyDebian {
		if _, err := p.Exec.Run(ctx, "a2enmod", "ssl"); err != nil {
			p.Log.Warn("apache: a2enmod ssl failed", "err", err)
		}
		if _, err := p.Exec.Run(ctx, "a2enconf", "ignition-ssl"); err != nil {
			p.Log.Warn("apache: a2enconf ignition-ssl failed", "err", err)
		}
	}

	p.Log.Info("apache: HTTPS vhost configured", "conf", confPath)
	return nil
}

// ConfigureApache configures httpd for serving ignition payloads over HTTPS:
// writes the TLS vhost conf, adjusts the port, enables the service, and
// creates the ignition directory. The payloads contain the cluster
// pull-secret, SSH authorized keys, and machine-config tokens; TLS with a
// pinned CA cert is the primary defence against credential capture over the
// machine-network VLAN — BuildIgnitionURLForNode enforces the RFC1918
// invariant at config time.
func (p *Phase) ConfigureApache(ctx context.Context, cfg *config.Config, projectRoot string) error {
	p.Log.Info("apache: configuring httpd for serving ignition files over https")

	bindIP := cfg.HTTPServer.IgnitionServerIP
	// Listen :443 is provided by the platform's mod_ssl default conf
	// (RHEL: /etc/httpd/conf.d/ssl.conf, Debian: ports.conf after a2enmod ssl).
	// 443 is already labeled https_port_t in the default RHEL SELinux policy,
	// so configureSELinuxForApache is intentionally not called here.

	webRoot := cfg.HTTPServer.Root
	if webRoot == "" {
		webRoot = phase.DefaultHTTPServerRoot
	}

	certPath, keyPath := IgnitionCertPaths(projectRoot)
	if err := p.configureApacheHTTPS(ctx, certPath, keyPath, webRoot, bindIP); err != nil {
		return &errtypes.ClusterError{Msg: "failed to configure apache HTTPS vhost", Err: err}
	}

	if err := enableAndStartApache(ctx, p.OS.ApacheServiceName()); err != nil {
		return &errtypes.ClusterError{Msg: "failed to enable and start apache", Err: err}
	}

	p.verifyApacheListening(ctx, bindIP)

	ignitionDir, err := p.ensureIgnitionDir(ctx, webRoot)
	if err != nil {
		return err
	}

	p.Log.Info("apache: ignition directory created", "path", ignitionDir)
	return nil
}

// DeployToWebServer copies the generated ignition files from clusterDir
// into the httpd web root. Auth credentials (kubeconfig, kubeadmin-password)
// are intentionally not copied here — they are consumed directly from
// clusterDir by the install and postinstall phases and must not be placed
// under the apache DocumentRoot.
func (p *Phase) DeployToWebServer(ctx context.Context, cfg *config.Config, clusterDir string) error {
	webRoot := cfg.HTTPServer.Root
	if webRoot == "" {
		webRoot = phase.DefaultHTTPServerRoot
	}

	ignitionDir, err := p.ensureIgnitionDir(ctx, webRoot)
	if err != nil {
		return err
	}

	for _, file := range ignitionFilenames {
		srcPath := filepath.Join(clusterDir, file)
		if !system.FileExists(srcPath) {
			continue
		}

		destPath := filepath.Join(ignitionDir, file)
		// 0o640: apache group readable only; ignition files carry pullSecret.
		if err := system.CopyFileMode(srcPath, destPath, 0o640); err != nil {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("failed to copy %s", file), Err: err}
		}
	}

	return nil
}

// VerifyWebServer fetches bootstrap.ign over HTTPS from baseURL and verifies
// the server certificate against caCertPEM. A mismatch causes the TLS handshake
// to fail — confirming Apache is serving the cert that was embedded into the
// node ISOs via --ignition-ca.
func (p *Phase) VerifyWebServer(ctx context.Context, baseURL string, caCertPEM []byte) error {
	testURL := fmt.Sprintf("%s/bootstrap.ign", baseURL)

	block, _ := pem.Decode(caCertPEM)
	if block == nil {
		return &errtypes.ConfigError{Msg: "ignition ca pem is not a valid PEM block"}
	}
	cert, parseErr := x509.ParseCertificate(block.Bytes)
	if parseErr != nil {
		return &errtypes.ConfigError{Msg: "failed to parse ignition ca cert", Err: parseErr}
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	client := httputil.NewWithCA(pool, httputil.TimeoutShort)

	p.Log.Info("apache: verifying https web server", "url", testURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, http.NoBody)
	if err != nil {
		return &errtypes.NetworkError{Msg: "failed to create request", Err: err}
	}

	resp, err := client.Do(req)
	if err != nil {
		return &errtypes.NetworkError{Msg: "failed to connect to web server", Err: err}
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
