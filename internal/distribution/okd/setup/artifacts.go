package setup

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
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
// p.BinDir (empty falls back to phase.DefaultBinDir) with executable mode.
func (p *Phase) InstallToolsToSystem(_ context.Context, srcDir string) error {
	binaries := []string{"openshift-install", "oc", "kubectl"}
	destDir := phase.BinDirOrDefault(p.BinDir)

	for _, binary := range binaries {
		srcPath := filepath.Join(srcDir, binary)
		if !system.FileExists(srcPath) {
			continue
		}

		destPath := filepath.Join(destDir, binary)

		if err := system.CopyFile(srcPath, destPath); err != nil {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("failed to install %s", binary), Err: err}
		}

		if err := system.MakeExecutable(destPath); err != nil {
			p.Log.Warn("tools: failed to set executable permission", "binary", binary, "err", err)
		}

		if !system.FileExists(destPath) {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("tools: %s not found at %s after install", binary, destPath)}
		}

		p.Log.Info("tools: installed binary", "binary", binary, "path", destPath)
	}

	return nil
}
