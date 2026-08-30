package setup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// ctxReader checks ctx once per chunk so io.Copy aborts promptly on cancellation.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (cr *ctxReader) Read(p []byte) (int, error) {
	if err := cr.ctx.Err(); err != nil {
		return 0, err
	}
	return cr.r.Read(p)
}

// fsyncDir durably persists a preceding rename, matching system.AtomicWrite's last step.
func fsyncDir(dir string) error {
	f, err := os.OpenFile(dir, os.O_RDONLY|syscall.O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

// releaseImageRef is the OKD release-image coordinate; version is appended per call.
const releaseImageRef = "quay.io/okd/scos-release:"

// okdToolsVersionFile is the BinDir sentinel used to detect a stale OKD tool version on resume.
const okdToolsVersionFile = ".okd-tools-version"

func okdToolsVersionPath(binDir string) string {
	return filepath.Join(binDir, okdToolsVersionFile)
}

// downloadToolsAlreadyDone reports whether binDir has every tool binary plus a
// matching version sentinel; read failures return false.
func downloadToolsAlreadyDone(binDir, version string) bool {
	for _, binary := range phase.OKDToolBinaries() {
		if !system.FileExists(filepath.Join(binDir, binary)) {
			return false
		}
	}
	data, err := os.ReadFile(okdToolsVersionPath(binDir))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == version
}

// DownloadOKDTools bootstraps oc, extracts openshift-install and oc from the
// OKD release image, and installs them to BinDir.
func (p *Phase) DownloadOKDTools(ctx context.Context, version string, opts *Options) error {
	if err := system.EnsureDir(opts.DownloadDir); err != nil {
		return &errtypes.ConfigError{Msg: "create download directory", Err: err}
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

	if err := p.InstallToolsToSystem(ctx, opts.DownloadDir); err != nil {
		return err
	}

	// Written only after every binary lands; a crash before this leaves the
	// sentinel stale and re-downloads.
	sentinel := okdToolsVersionPath(config.BinDirOrDefault(p.BinDir))
	if err := system.AtomicWriteString(sentinel, version+"\n", 0o644); err != nil {
		return &errtypes.ConfigError{Msg: "write tools version sentinel", Err: err}
	}
	return nil
}

// InstallToolsToSystem copies the OKD binaries from srcDir into p.BinDir
// (or config.DefaultBinDir) with executable mode.
func (p *Phase) InstallToolsToSystem(ctx context.Context, srcDir string) error {
	binaries := phase.OKDToolBinaries()
	destDir := config.BinDirOrDefault(p.BinDir)

	for _, binary := range binaries {
		if err := ctx.Err(); err != nil {
			return err
		}

		srcPath := filepath.Join(srcDir, binary)
		if !system.FileExists(srcPath) {
			continue
		}

		if err := atomicInstallFile(ctx, srcPath, destDir, binary); err != nil {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("install %s", binary), Err: err}
		}

		p.Log.Info("tools: installed binary", "tool", binary, "path", filepath.Join(destDir, binary))
	}

	return nil
}

// atomicInstallFile requires name to be a bare file name; a separator or ".."
// would let it escape destDir.
func atomicInstallFile(ctx context.Context, src, destDir, name string) error {
	if name != filepath.Base(name) || !filepath.IsLocal(name) {
		return fmt.Errorf("install name %q is not a bare file name", name)
	}
	dst := filepath.Join(destDir, name)

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

	if _, err := io.Copy(tmp, &ctxReader{ctx: ctx, r: in}); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("copy %s: %w", src, err)
	}
	if err := tmp.Chmod(0o755); err != nil {
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
	if err := fsyncDir(filepath.Dir(dst)); err != nil {
		return fmt.Errorf("sync destination directory: %w", err)
	}
	return nil
}
