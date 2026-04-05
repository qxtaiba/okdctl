package setup

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/httputil"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

const (
	minIgnitionFileSize = 1000 // bytes
)

func (p *Phase) ensureIgnitionDir(ctx context.Context, webRoot string) (string, error) {
	ignitionDir := filepath.Join(webRoot, "ignition")

	if err := system.MkdirAll(ctx, ignitionDir, "ignition directory"); err != nil {
		return "", utils.WrapError("failed to create ignition directory", err)
	}

	if err := system.Chown(ctx, ignitionDir, "apache:apache", "ignition directory ownership"); err != nil {
		p.Log.Warn(fmt.Sprintf("apache: failed to set ignition dir ownership: %v", err))
	}
	if err := system.Chmod(ctx, ignitionDir, "755", "ignition directory permissions"); err != nil {
		p.Log.Warn(fmt.Sprintf("apache: failed to set ignition dir permissions: %v", err))
	}

	return ignitionDir, nil
}

func (p *Phase) configureApachePort(ctx context.Context) {
	httpdConf := "/etc/httpd/conf/httpd.conf"
	if !system.FileExists(httpdConf) {
		return
	}

	backupPath := fmt.Sprintf("%s.backup.%d", httpdConf, time.Now().Unix())
	if err := system.CopyFileWithElevation(ctx, httpdConf, backupPath, "httpd.conf backup"); err != nil {
		p.Log.Warn(fmt.Sprintf("apache: could not backup httpd.conf: %v", err))
	}

	result, err := p.Exec.Run(ctx, "sudo", "sed", "-i", "s/^Listen 80$/Listen 8080/", httpdConf)
	if err != nil || result.ExitCode != 0 {
		p.Log.Warn("apache: could not modify httpd.conf to listen on port 8080")
	}
}

func (p *Phase) configureSELinuxForApache(ctx context.Context) {
	if !executor.CommandExists("semanage") {
		return
	}
	// Try -a first; if port already exists, -m modifies the existing entry
	_, _ = p.Exec.Run(ctx, "sudo", "semanage", "port", "-a", "-t", "http_port_t", "-p", "tcp", "8080")
	_, _ = p.Exec.Run(ctx, "sudo", "semanage", "port", "-m", "-t", "http_port_t", "-p", "tcp", "8080")
}

func enableAndStartApache(ctx context.Context) error {
	if err := system.ManageService(ctx, system.ServiceEnable, "httpd", "apache httpd service"); err != nil {
		return utils.WrapError("failed to enable httpd", err)
	}
	if err := system.ManageService(ctx, system.ServiceStart, "httpd", "apache httpd service"); err != nil {
		return utils.WrapError("failed to start httpd", err)
	}
	if !system.IsServiceActive("httpd") {
		return fmt.Errorf("apache httpd service failed to start - check systemctl status httpd")
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

	if err := enableAndStartApache(ctx); err != nil {
		return err
	}

	p.verifyApacheListening(ctx)

	webRoot := cfg.HTTPServer.Root
	if webRoot == "" {
		webRoot = "/var/www/html"
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
		webRoot = "/var/www/html"
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
			return utils.WrapErrorf(err, "failed to copy %s", file)
		}

		if err := system.Chmod(ctx, destPath, "644", fmt.Sprintf("%s permissions", file)); err != nil {
			return utils.WrapErrorf(err, "failed to set permissions on %s", file)
		}
	}

	authSrc := filepath.Join(clusterDir, "auth")
	if system.FileExists(authSrc) {
		authDest := filepath.Join(webRoot, "auth")
		if result, err := p.Exec.Run(ctx, "sudo", "cp", "-r", authSrc, authDest); err != nil || result.ExitCode != 0 {
			p.Log.Warn("apache: could not copy auth directory to web root")
		}
	}

	return nil
}

func (p *Phase) VerifyWebServer(ctx context.Context, baseURL string) error {
	testURL := fmt.Sprintf("%s/bootstrap.ign", baseURL)

	client := httputil.NewAPIClient()

	p.Log.Info(fmt.Sprintf("apache: verifying web server at %s", testURL))

	resp, err := client.Get(testURL)
	if err != nil {
		return utils.WrapError("failed to connect to web server", err)
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
