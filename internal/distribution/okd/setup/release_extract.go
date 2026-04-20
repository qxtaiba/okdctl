package setup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/download"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/fetchplan"
	"github.com/qxtaiba/okdctl/internal/system"
)

const ocExtractTimeout = 120 * time.Second

// bootstrapOC ensures oc is available in downloadDir. If already present the
// cached binary is reused. The fetch URL is routed through resolver so M24 can
// redirect it. No upstream checksum is published for the bootstrap-oc URL;
// binary-exists+nonzero-size is the integrity gate. Final binaries come from
// the digest-pinned release image, so a tampered bootstrap oc would still fail
// downstream digest verification at extract time.
func (p *Phase) bootstrapOC(ctx context.Context, downloadDir string, resolver fetchplan.Resolver) (string, error) {
	ocPath := filepath.Join(downloadDir, "oc")
	if system.FileExists(ocPath) {
		p.Log.Info("tools: bootstrap oc already present", "path", ocPath)
		return ocPath, nil
	}

	plan := fetchplan.BuildM22BootstrapOCPlan()
	url, err := resolver.ResolveBlob(plan.HTTPS[0])
	if err != nil {
		return "", &errtypes.ConfigError{Msg: "failed to resolve bootstrap oc URL", Err: err}
	}

	archivePath := filepath.Join(downloadDir, "oc.tar.gz")
	p.Log.Info("tools: fetching bootstrap oc", "url", url)

	if err := download.Download(ctx, &download.Options{
		URL:         url,
		OutputPath:  archivePath,
		Description: "bootstrap-oc",
		Timeout:     3 * time.Minute,
		Logger:      p.Log,
	}); err != nil {
		return "", &errtypes.NetworkError{Msg: "failed to download bootstrap oc", Err: err}
	}

	if err := download.ExtractTarGz(ctx, download.ExtractOptions{
		ArchivePath:    archivePath,
		DestDir:        downloadDir,
		CleanupArchive: true,
		Logger:         p.Log,
	}); err != nil {
		return "", &errtypes.NetworkError{Msg: "failed to extract bootstrap oc", Err: err}
	}

	fi, statErr := os.Stat(ocPath)
	if statErr != nil {
		return "", &errtypes.NetworkError{Msg: "bootstrap oc binary missing after extract", Err: statErr}
	}
	if fi.Size() == 0 {
		return "", &errtypes.NetworkError{Msg: "bootstrap oc binary is empty after extract"}
	}

	if err := system.MakeExecutable(ocPath); err != nil {
		p.Log.Warn("tools: failed to chmod bootstrap oc", "err", err)
	}

	p.Log.Info("tools: bootstrap oc ready", "path", ocPath)
	return ocPath, nil
}

// extractReleaseImage runs `oc adm release extract --tools <ref> --to <destDir>`
// with a bounded timeout. Registry auth failures produce *errtypes.AuthError;
// other failures produce *errtypes.ClusterError.
func (p *Phase) extractReleaseImage(ctx context.Context, ocPath, ref, destDir string) error {
	extractCtx, cancel := context.WithTimeout(ctx, ocExtractTimeout)
	defer cancel()

	cmd := exec.CommandContext(extractCtx, ocPath,
		"adm", "release", "extract",
		"--tools", ref,
		"--to", destDir,
	)

	var stderr strings.Builder
	cmd.Stderr = &stderr

	if runErr := cmd.Run(); runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		p.Log.Error("tools: oc adm release extract failed", "ref", ref, "stderr", msg)
		if strings.Contains(msg, "unauthorized") || strings.Contains(msg, "authentication") {
			return &errtypes.AuthError{Msg: fmt.Sprintf("release extract: registry auth failed for %s", ref), Err: runErr}
		}
		return &errtypes.ClusterError{Msg: fmt.Sprintf("release extract failed for %s", ref), Err: runErr}
	}

	return extractReleaseTarballs(ctx, destDir, p.Log)
}

// extractReleaseTarballs extracts the versioned tarballs that
// `oc adm release extract --tools` places in destDir so that
// InstallToolsToSystem finds the bare binaries.
func extractReleaseTarballs(ctx context.Context, destDir string, logger *slog.Logger) error {
	matches, err := filepath.Glob(filepath.Join(destDir, "*.tar.gz"))
	if err != nil {
		return fmt.Errorf("glob release tarballs: %w", err)
	}
	for _, archivePath := range matches {
		if err := download.ExtractTarGz(ctx, download.ExtractOptions{
			ArchivePath:    archivePath,
			DestDir:        destDir,
			CleanupArchive: true,
		}); err != nil {
			return fmt.Errorf("extract %s: %w", filepath.Base(archivePath), err)
		}
		logger.Info("tools: extracted release tarball", "file", filepath.Base(archivePath))
	}
	return nil
}
