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
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/system"
)

const (
	minIgnitionFileSize = 1000 // bytes
)

func (p *Phase) ensureIgnitionDir(_ context.Context, webRoot string) (string, error) {
	ignitionDir := filepath.Join(webRoot, "ignition")

	if err := os.MkdirAll(ignitionDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create ignition directory: %w", err)
	}

	apacheUser := p.OS.ApacheUser()
	if err := system.ChownByName(ignitionDir, apacheUser+":"+apacheUser); err != nil {
		p.Log.Warn("apache: failed to set ignition dir ownership", "err", err)
	}
	// Explicit chmod in case ignitionDir pre-existed with narrower perms.
	if err := os.Chmod(ignitionDir, 0o755); err != nil {
		p.Log.Warn("apache: failed to set ignition dir permissions", "err", err)
	}

	return ignitionDir, nil
}

func (p *Phase) configureApachePort(_ context.Context) {
	httpdConf := p.OS.ApacheConfigPath()
	if !system.FileExists(httpdConf) {
		return
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
			line = "Listen 8080"
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
		p.Log.Warn("apache: could not modify httpd.conf to listen on port 8080", "err", err)
	}
}

func (p *Phase) configureSELinuxForApache(ctx context.Context) {
	if !p.OS.HasSELinux() {
		return
	}
	if !executor.CommandExists("semanage") {
		return
	}
	// Try -a first; if port already exists, -m modifies the existing entry
	_, _ = p.Exec.Run(ctx, "semanage", "port", "-a", "-t", "http_port_t", "-p", "tcp", "8080")
	_, _ = p.Exec.Run(ctx, "semanage", "port", "-m", "-t", "http_port_t", "-p", "tcp", "8080")
}

func enableAndStartApache(ctx context.Context, serviceName string) error {
	if err := system.ManageService(ctx, system.ServiceEnable, serviceName, "apache service"); err != nil {
		return fmt.Errorf("failed to enable %s: %w", serviceName, err)
	}
	if err := system.ManageService(ctx, system.ServiceStart, serviceName, "apache service"); err != nil {
		return fmt.Errorf("failed to start %s: %w", serviceName, err)
	}
	if !system.IsServiceActive(ctx, serviceName) {
		return fmt.Errorf("apache service %s failed to start - check systemctl status %s", serviceName, serviceName)
	}
	return nil
}

func (p *Phase) verifyApacheListening(ctx context.Context) {
	dialer := &net.Dialer{Timeout: 1 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", "127.0.0.1:8080")
	if err != nil {
		p.Log.Warn("apache: httpd may not be listening on port 8080 - check configuration")
		return
	}
	_ = conn.Close()
	p.Log.Info("apache: httpd service listening on port 8080")
}

// ConfigureApache configures httpd for serving ignition payloads: port,
// SELinux context, service enable, and ignition directory creation.
func (p *Phase) ConfigureApache(ctx context.Context, cfg *config.Config) error {
	p.Log.Info("apache: configuring httpd for serving ignition files")

	p.configureApachePort(ctx)
	p.configureSELinuxForApache(ctx)

	if err := enableAndStartApache(ctx, p.OS.ApacheServiceName()); err != nil {
		return err
	}

	p.verifyApacheListening(ctx)

	webRoot := cfg.HTTPServer.Root
	if webRoot == "" {
		webRoot = phase.DefaultHTTPServerRoot
	}
	ignitionDir, err := p.ensureIgnitionDir(ctx, webRoot)
	if err != nil {
		return err
	}

	p.Log.Info(fmt.Sprintf("apache: ignition directory created at %s", ignitionDir))
	return nil
}

// DeployToWebServer copies the generated ignition files and the auth
// directory (kubeconfig, kubeadmin-password) from clusterDir into the
// httpd web root, preserving file modes so sensitive files stay protected.
func (p *Phase) DeployToWebServer(ctx context.Context, cfg *config.Config, clusterDir string) error {
	webRoot := cfg.HTTPServer.Root
	if webRoot == "" {
		webRoot = phase.DefaultHTTPServerRoot
	}

	ignitionDir, err := p.ensureIgnitionDir(ctx, webRoot)
	if err != nil {
		return err
	}

	ignitionFiles := []string{"bootstrap.ign", "master.ign", "worker.ign"}
	for _, file := range ignitionFiles {
		srcPath := filepath.Join(clusterDir, file)
		if !system.FileExists(srcPath) {
			continue
		}

		destPath := filepath.Join(ignitionDir, file)
		if err := system.CopyFile(srcPath, destPath); err != nil {
			return fmt.Errorf("failed to copy %s: %w", file, err)
		}

		if err := os.Chmod(destPath, 0o644); err != nil {
			return fmt.Errorf("failed to set permissions on %s: %w", file, err)
		}
	}

	authSrc := filepath.Join(clusterDir, "auth")
	if system.FileExists(authSrc) {
		authDest := filepath.Join(webRoot, "auth")
		if err := copyAuthTree(authSrc, authDest); err != nil {
			return fmt.Errorf("failed to copy auth directory %s to web root %s: %w", authSrc, authDest, err)
		}
	}

	return nil
}

// copyAuthTree copies the install-config auth/ directory (kubeadmin-password,
// kubeconfig) into the web root, preserving each file's mode bits. `cp -r`
// would lose mode because it doesn't imply `-p`, leaving the htpasswd-class
// files world-readable under the apache user's umask.
func copyAuthTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode().Perm())
		}
		return system.CopyFile(path, target)
	})
}

// VerifyWebServer fetches bootstrap.ign from baseURL and checks the response
// status and approximate size to catch misconfigured or empty deploys early.
func (p *Phase) VerifyWebServer(ctx context.Context, baseURL string) error {
	testURL := fmt.Sprintf("%s/bootstrap.ign", baseURL)

	client := httputil.New(httputil.TimeoutShort)

	p.Log.Info(fmt.Sprintf("apache: verifying web server at %s", testURL))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to web server: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("web server returned status %d for %s", resp.StatusCode, testURL)
	}

	if resp.ContentLength > 0 && resp.ContentLength < minIgnitionFileSize {
		return fmt.Errorf("bootstrap.ign appears too small (%d bytes)", resp.ContentLength)
	}

	p.Log.Info("apache: web server accessible and serving ignition files")

	return nil
}
