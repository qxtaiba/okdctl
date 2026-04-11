package setup

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/utils/httputil"
	"github.com/qxtaiba/okdctl/internal/utils/system"
)

const (
	minIgnitionFileSize = 1000 // bytes
)

func (p *Phase) ensureIgnitionDir(ctx context.Context, webRoot string) (string, error) {
	ignitionDir := filepath.Join(webRoot, "ignition")

	if err := system.MkdirAll(ctx, ignitionDir, "ignition directory"); err != nil {
		return "", fmt.Errorf("failed to create ignition directory: %w", err)
	}

	apacheUser := p.OS.ApacheUser()
	if err := system.Chown(ctx, ignitionDir, apacheUser+":"+apacheUser, "ignition directory ownership"); err != nil {
		p.Log.Warn(fmt.Sprintf("apache: failed to set ignition dir ownership: %v", err))
	}
	if err := system.Chmod(ctx, ignitionDir, "755", "ignition directory permissions"); err != nil {
		p.Log.Warn(fmt.Sprintf("apache: failed to set ignition dir permissions: %v", err))
	}

	return ignitionDir, nil
}

func (p *Phase) configureApachePort(ctx context.Context) {
	httpdConf := p.OS.ApacheConfigPath()
	if !system.FileExists(httpdConf) {
		return
	}

	backupPath := fmt.Sprintf("%s.backup.%d", httpdConf, time.Now().Unix())
	if err := system.CopyFileWithElevation(ctx, httpdConf, backupPath, "httpd.conf backup"); err != nil {
		p.Log.Warn(fmt.Sprintf("apache: could not backup httpd.conf: %v", err))
	}

	result, err := p.Exec.Run(ctx, "sudo", "sed", "-i", "s/^Listen 80$/Listen 8080/", httpdConf)
	if err != nil || result.ExitCode != 0 {
		p.Log.Warn(fmt.Sprintf("apache: could not modify httpd.conf to listen on port 8080: %v", err))
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
	_, _ = p.Exec.Run(ctx, "sudo", "semanage", "port", "-a", "-t", "http_port_t", "-p", "tcp", "8080")
	_, _ = p.Exec.Run(ctx, "sudo", "semanage", "port", "-m", "-t", "http_port_t", "-p", "tcp", "8080")
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
	result, _ := p.Exec.Run(ctx, "ss", "-tlnp")
	if result != nil && result.ExitCode == 0 && strings.Contains(result.Stdout, ":8080 ") {
		p.Log.Info("apache: httpd service listening on port 8080")
	} else {
		p.Log.Warn("apache: httpd may not be listening on port 8080 - check configuration")
	}
}

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
		if err := system.CopyFileWithElevation(ctx, srcPath, destPath, fmt.Sprintf("ignition file %s", file)); err != nil {
			return fmt.Errorf("failed to copy %s: %w", file, err)
		}

		if err := system.Chmod(ctx, destPath, "644", fmt.Sprintf("%s permissions", file)); err != nil {
			return fmt.Errorf("failed to set permissions on %s: %w", file, err)
		}
	}

	authSrc := filepath.Join(clusterDir, "auth")
	if system.FileExists(authSrc) {
		authDest := filepath.Join(webRoot, "auth")
		_, err := p.Exec.RunChecked(ctx, "sudo", "cp", "-r", authSrc, authDest)
		if err != nil {
			return fmt.Errorf("failed to copy auth directory %s to web root %s: %w", authSrc, authDest, err)
		}
	}

	return nil
}

func (p *Phase) VerifyWebServer(ctx context.Context, baseURL string) error {
	testURL := fmt.Sprintf("%s/bootstrap.ign", baseURL)

	client := httputil.NewAPIClient()

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
