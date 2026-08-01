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

// ctxReader aborts an in-flight io.Copy within one chunk once ctx is done,
// mirroring download.ctxReader so a cancelled deploy does not wait out a
// multi-hundred-MB binary copy.
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

// fsyncDir fsyncs dir so the directory-entry update from a preceding rename is
// crash-durable, matching system.AtomicWrite's final step.
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

// releaseImageRef is the canonical OKD release-image coordinate used by
// `oc adm release extract --tools`. The :<version> tag is appended per call.
const releaseImageRef = "quay.io/okd/scos-release:"

// okdToolsVersionFile is the BinDir sentinel recording which OKD release the
// installed binaries were extracted from. The binaries themselves are
// version-blind to a resume guard, so download-tools' AlreadyDone compares
// this sentinel against the configured version — without it, a resume after
// a version change would silently reuse stale tools.
const okdToolsVersionFile = ".okd-tools-version"

func okdToolsVersionPath(binDir string) string {
	return filepath.Join(binDir, okdToolsVersionFile)
}

// downloadToolsAlreadyDone reports whether every OKD tool binary exists in
// binDir and the version sentinel matches version. Any read failure
// conservatively returns false so Exec re-downloads and surfaces the real
// failure.
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
// OKD release container image, and installs them to BinDir.
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

	// Written only after every binary landed: a crash before this point leaves
	// the sentinel stale or absent, so the guard re-runs the download.
	sentinel := okdToolsVersionPath(config.BinDirOrDefault(p.BinDir))
	if err := system.AtomicWriteString(sentinel, version+"\n", 0o644); err != nil {
		return &errtypes.ConfigError{Msg: "write tools version sentinel", Err: err}
	}
	return nil
}

// InstallToolsToSystem copies the downloaded OKD binaries from srcDir into
// p.BinDir (empty falls back to config.DefaultBinDir) with executable mode.
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

// atomicInstallFile streams src into a temp file created inside destDir,
// applies executable mode, fsyncs the file and the destination directory, and
// renames it to destDir/name. Installed binaries back resume guards that never
// checksum content (download-tools' AlreadyDone checks presence plus the
// version sentinel), so a crash mid-install must never leave a truncated file
// at the destination. Streaming avoids buffering multi-hundred-MB release
// binaries the way system.AtomicWrite would; the copy is ctx-aware so a
// cancelled deploy aborts an in-flight binary. name must be a bare file name —
// anything carrying a separator or ".." cannot escape destDir.
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
