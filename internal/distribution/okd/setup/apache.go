package setup

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/logutil"
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

func (p *Phase) configureApachePort(ctx context.Context, bindIP string) {
	if err := ctx.Err(); err != nil {
		return
	}
	httpdConf := p.OS.ApacheConfigPath()
	if !system.FileExists(httpdConf) {
		return
	}

	listenDirective := "Listen 8080"
	if bindIP != "" {
		// Bind to the bastion bridge IP only; ignition files carry the pull-secret
		// and must not be served on every interface during the bootstrap window.
		listenDirective = fmt.Sprintf("Listen %s:8080", bindIP)
	}

	backupPath := fmt.Sprintf("%s.backup.%d", httpdConf, time.Now().Unix())
	if err := system.CopyFile(httpdConf, backupPath); err != nil {
		p.Log.Warn("apache: could not backup httpd.conf", "err", err)
	}

	original, err := os.ReadFile(httpdConf)
	if err != nil {
		p.Log.Warn("apache: could not read httpd.conf", "err", err)
		return
	}

	var buf bytes.Buffer
	changed := false
	scanner := bufio.NewScanner(bytes.NewReader(original))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "Listen 80" {
			line = listenDirective
			changed = true
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		p.Log.Warn("apache: scan of httpd.conf failed", "err", err)
		return
	}
	if !changed {
		return
	}
	if err := system.AtomicWrite(httpdConf, buf.Bytes(), 0o644); err != nil {
		p.Log.Warn("apache: could not modify httpd.conf", "err", err)
	}
}

func (p *Phase) configureSELinuxForApache(ctx context.Context) {
	if !p.OS.HasSELinux() {
		return
	}
	if !executor.CommandExists("semanage") {
		return
	}
	// Try -a first; if the port label already exists semanage exits non-zero
	// and we fall through to -m which modifies it. The -m result is the one
	// that determines final state — log it at warn level so a "policy not
	// loaded" / SELinux-disabled / missing-perm failure isn't invisible.
	_, _ = p.Exec.Run(ctx, "semanage", "port", "-a", "-t", "http_port_t", "-p", "tcp", "8080")
	if r, err := p.Exec.Run(ctx, "semanage", "port", "-m", "-t", "http_port_t", "-p", "tcp", "8080"); err != nil {
		p.Log.Warn("apache: semanage port modify failed", "err", err)
	} else if r.ExitCode != 0 {
		p.Log.Warn("apache: semanage port modify exited non-zero",
			"exit", r.ExitCode, "stderr", logutil.RedactableStderr(strings.TrimSpace(r.Stderr)))
	}
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
	addr := "127.0.0.1:8080"
	if bindIP != "" {
		addr = net.JoinHostPort(bindIP, "8080")
	}
	dialer := &net.Dialer{Timeout: 1 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		p.Log.Warn("apache: httpd may not be listening on port 8080 - check configuration")
		return
	}
	_ = conn.Close()
	p.Log.Info("apache: httpd service listening on port 8080")
}

// ConfigureApache configures httpd for serving ignition payloads: port,
// SELinux context, service enable, and ignition directory creation. The
// payloads contain the cluster pull-secret, SSH authorized keys, and
// machine-config tokens; confidentiality depends on the bastion being
// isolated to a private VLAN — BuildIgnitionURLForNode enforces the
// RFC1918 invariant at config time.
func (p *Phase) ConfigureApache(ctx context.Context, cfg *config.Config) error {
	p.Log.Info("apache: configuring httpd for serving ignition files")

	bindIP := cfg.HTTPServer.IgnitionServerIP
	p.configureApachePort(ctx, bindIP)
	p.configureSELinuxForApache(ctx)

	if err := enableAndStartApache(ctx, p.OS.ApacheServiceName()); err != nil {
		return &errtypes.ClusterError{Msg: "failed to enable and start apache", Err: err}
	}

	p.verifyApacheListening(ctx, bindIP)

	webRoot := cfg.HTTPServer.Root
	if webRoot == "" {
		webRoot = phase.DefaultHTTPServerRoot
	}
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

// VerifyWebServer fetches bootstrap.ign from baseURL and checks the response
// status and approximate size to catch misconfigured or empty deploys early.
func (p *Phase) VerifyWebServer(ctx context.Context, baseURL string) error {
	testURL := fmt.Sprintf("%s/bootstrap.ign", baseURL)

	client := httputil.New(httputil.TimeoutShort)

	p.Log.Info("apache: verifying web server", "url", testURL)

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

	p.Log.Info("apache: web server accessible and serving ignition files")

	return nil
}
