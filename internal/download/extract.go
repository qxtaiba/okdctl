package download

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// extractConfig holds the resolved configuration for an ExtractTarGz call.
// logger is normalised via logutil.OrNop once at ExtractTarGz construction.
type extractConfig struct {
	archivePath      string
	destDir          string
	expectedChecksum string
	stripComponents  int
	cleanupArchive   bool
	logger           *slog.Logger
}

// ExtractOption configures an ExtractTarGz call.
type ExtractOption func(*extractConfig)

// ErrSymlinkEscape reports a symlink whose resolved location or target lands
// outside destDir. Callers can match it with errors.Is.
var ErrSymlinkEscape = errors.New("symlink resolves outside destination")

// WithExtractChecksum sets the expected SHA-256 hex digest for the archive; empty disables verification.
func WithExtractChecksum(sum string) ExtractOption {
	return func(c *extractConfig) { c.expectedChecksum = sum }
}

// WithExtractStripComponents removes n leading path components from archive entries, like tar --strip-components.
func WithExtractStripComponents(n int) ExtractOption {
	return func(c *extractConfig) { c.stripComponents = n }
}

// WithExtractCleanupArchive removes the archive file after successful extraction.
func WithExtractCleanupArchive(v bool) ExtractOption {
	return func(c *extractConfig) { c.cleanupArchive = v }
}

// WithExtractLogger injects a structured logger; nil falls back to logutil.NopLogger.
func WithExtractLogger(l *slog.Logger) ExtractOption {
	return func(c *extractConfig) { c.logger = logutil.OrNop(l) }
}

// verifyResolvedLink resolves rel (a path relative to destDir) through the
// real filesystem and rejects it if the result escapes destDir. rel names a
// just-created symlink, so EvalSymlinks follows both every parent component and
// the link itself; a target that cannot be resolved within destDir — dangling
// or escaping — is rejected identically, matching the pre-os.Root
// verifyResolvedPath the ISO/tool archives were validated against.
func verifyResolvedLink(destDir, rel string) error {
	cleanDest, err := filepath.EvalSymlinks(destDir)
	if err != nil {
		cleanDest = filepath.Clean(destDir)
	}
	linkPath := filepath.Join(destDir, rel)
	resolved, err := filepath.EvalSymlinks(linkPath)
	if err != nil {
		return fmt.Errorf("%w: resolve %s: %w", ErrSymlinkEscape, linkPath, err)
	}
	if resolved != cleanDest && !strings.HasPrefix(resolved, cleanDest+string(os.PathSeparator)) {
		return fmt.Errorf("%w: %s -> %s", ErrSymlinkEscape, linkPath, resolved)
	}
	return nil
}

// processTarEntry writes a single tar entry through root, which scopes every
// filesystem op to destDir via openat2 so the kernel — not a textual prefix
// check — rejects any resolved path outside the tree for files and dirs.
//
// Symlinks need two extra layers. os.Root will happily create a link whose
// stored target text escapes destDir (it validates traversal *through* a link,
// not the text written into one), and the extracted tree is later walked by
// non-Root code that has no such protection. Worse, two composed in-tree links
// can escape even though each passes a textual check: `a/b/toroot -> ../..`
// resolves to destDir, then `a/b/toroot/esc -> ../etc` is created *through*
// toroot and lands at destDir/esc pointing outside — the textual check saw the
// literal parent "a/b/toroot", not the resolved one. So symlinks get (1) a
// textual pre-check that rejects absolute or single-link escaping targets
// before creation, and (2) a resolution post-check (EvalSymlinks on the real
// filesystem) that rejects and removes any link whose resolved location or
// target lands outside destDir once every parent link has been followed.
func processTarEntry(root *os.Root, tarReader *tar.Reader, header *tar.Header, stripComponents int) error {
	name := header.Name
	if stripComponents > 0 {
		parts := strings.Split(name, "/")
		if len(parts) <= stripComponents {
			return nil
		}
		name = strings.Join(parts[stripComponents:], "/")
	}

	if name == "" {
		return nil
	}

	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("archive entry attempts to escape destination: %s", name)
	}

	switch header.Typeflag {
	case tar.TypeDir:
		if err := root.MkdirAll(clean, os.FileMode(header.Mode&0o755)); err != nil {
			return fmt.Errorf("create directory %s: %w", name, err)
		}

	case tar.TypeReg:
		if parent := filepath.Dir(clean); parent != "." {
			if err := root.MkdirAll(parent, 0o755); err != nil {
				return fmt.Errorf("create parent directory for %s: %w", name, err)
			}
		}

		outFile, err := root.OpenFile(clean, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode&0o755))
		if err != nil {
			return fmt.Errorf("create file %s: %w", name, err)
		}

		if _, err := io.Copy(outFile, tarReader); err != nil {
			_ = outFile.Close()
			return fmt.Errorf("write file %s: %w", name, err)
		}
		if err := outFile.Close(); err != nil {
			return fmt.Errorf("close extracted file %s: %w", name, err)
		}

	case tar.TypeSymlink:
		linkTarget := header.Linkname
		if filepath.IsAbs(linkTarget) {
			return fmt.Errorf("absolute symlink target not allowed: %s -> %s", name, linkTarget)
		}

		resolvedTarget := filepath.Clean(filepath.Join(filepath.Dir(clean), linkTarget)) //nolint:gosec // G305: validated below
		if resolvedTarget == ".." || strings.HasPrefix(resolvedTarget, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("symlink target escapes destination: %s -> %s", name, linkTarget)
		}

		if err := root.Symlink(linkTarget, clean); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return fmt.Errorf("create symlink %s: %w", name, err)
			}
			if err := root.Remove(clean); err != nil {
				return fmt.Errorf("remove existing symlink %s: %w", name, err)
			}
			if err := root.Symlink(linkTarget, clean); err != nil {
				return fmt.Errorf("replace symlink %s: %w", name, err)
			}
		}

		if err := verifyResolvedLink(root.Name(), clean); err != nil {
			// The link escaped (or resolved to a target unreachable within
			// destDir) once its parent components were followed. Remove it so
			// no later non-Root consumer can traverse the escaping link, then
			// fail the whole extraction.
			_ = root.Remove(clean)
			return fmt.Errorf("symlink %s: %w", name, err)
		}

	default:
		// Skip unsupported entry types (hardlinks, char/block devices, FIFOs).
	}

	return nil
}

// ExtractTarGz extracts archivePath into destDir. File and directory writes
// are scoped to destDir through an os.Root, so zip-slip escapes are rejected by
// the kernel at open time. Symlinks additionally get a resolution post-check
// (see processTarEntry) so a stored target that escapes destDir is rejected and
// removed before any non-Root consumer walks the tree.
func ExtractTarGz(ctx context.Context, archivePath, destDir string, opts ...ExtractOption) error {
	cfg := &extractConfig{
		archivePath: archivePath,
		destDir:     destDir,
		logger:      logutil.NopLogger,
	}
	for _, o := range opts {
		o(cfg)
	}

	filename := filepath.Base(archivePath)

	if cfg.expectedChecksum != "" {
		cfg.logger.Info("download: validating sha256 checksum", "file", filename)

		if err := ValidateChecksum(ctx, archivePath, cfg.expectedChecksum); err != nil {
			return fmt.Errorf("checksum validation failed for %s: %w", filename, err)
		}

		cfg.logger.Info("download: checksum validated", "file", filename)
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive %s: %w", archivePath, err)
	}
	defer func() { _ = file.Close() }()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("decompress archive %s: %w", archivePath, err)
	}
	defer func() { _ = gzipReader.Close() }()

	tarReader := tar.NewReader(gzipReader)

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	root, err := os.OpenRoot(destDir)
	if err != nil {
		return fmt.Errorf("open destination root %s: %w", destDir, err)
	}
	defer func() { _ = root.Close() }()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry from %s: %w", archivePath, err)
		}

		if err := processTarEntry(root, tarReader, header, cfg.stripComponents); err != nil {
			return err
		}
	}

	if cfg.cleanupArchive {
		if err := os.Remove(archivePath); err != nil {
			cfg.logger.Warn("download: failed to cleanup archive", "file", filename)
		}
	}

	cfg.logger.Info("download: extracted", "file", filename)

	return nil
}
