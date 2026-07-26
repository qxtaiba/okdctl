package setup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// releaseImageRef is the canonical OKD release-image coordinate used by
// `oc adm release extract --tools`. The :<version> tag is appended per call.
const releaseImageRef = "quay.io/okd/scos-release:"

// DownloadOKDTools bootstraps oc, extracts openshift-install and oc from the
// OKD release container image, and installs them to BinDir.
func (p *Phase) DownloadOKDTools(ctx context.Context, version string, opts *Options) error {
	if err := system.EnsureDir(opts.DownloadDir); err != nil {
		return &errtypes.ConfigError{Msg: "failed to create download directory", Err: err}
	}

	ocPath, err := p.bootstrapOC(ctx, opts.DownloadDir)
	if err != nil {
		return err
	}

	ref := releaseImageRef + version
	p.Log.Info("tools: extracting OKD binaries from release image", "ref", ref)

	if err := p.extractReleaseImage(ctx, ocPath, ref, opts.DownloadDir); err != nil {
		return err
	}

	return p.InstallToolsToSystem(ctx, opts.DownloadDir)
}

// InstallToolsToSystem copies the downloaded OKD binaries from srcDir into
// p.BinDir (empty falls back to config.DefaultBinDir) with executable mode.
func (p *Phase) InstallToolsToSystem(ctx context.Context, srcDir string) error {
	binaries := []string{"openshift-install", "oc", "kubectl"}
	destDir := config.BinDirOrDefault(p.BinDir)

	for _, binary := range binaries {
		if err := ctx.Err(); err != nil {
			return err
		}

		srcPath := filepath.Join(srcDir, binary)
		if !system.FileExists(srcPath) {
			continue
		}

		destPath := filepath.Join(destDir, binary)

		if err := atomicInstallFile(srcPath, destPath, 0o755); err != nil {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("failed to install %s", binary), Err: err}
		}

		p.Log.Info("tools: installed binary", "binary", binary, "path", destPath)
	}

	return nil
}

// atomicInstallFile streams src into a temp file created beside dst, applies
// mode, fsyncs, and renames it into place. Installed binaries back
// existence-only resume guards (download-tools' AlreadyDone), so a crash
// mid-install must never leave a truncated file at dst. Streaming avoids
// buffering multi-hundred-MB release binaries the way system.AtomicWrite
// would.
func atomicInstallFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	if err := system.EnsureDirForFile(dst); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("copy %s: %w", src, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		return fmt.Errorf("rename into place: %w", err)
	}
	return nil
}
