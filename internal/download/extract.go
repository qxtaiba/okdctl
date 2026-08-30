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

type extractConfig struct {
	stripComponents int
	cleanupArchive  bool
	logger          *slog.Logger
}

// ExtractOption configures an ExtractTarGz call.
type ExtractOption func(*extractConfig)

// ErrSymlinkEscape reports a symlink that resolves outside destDir. Match it with errors.Is.
var ErrSymlinkEscape = errors.New("symlink resolves outside destination")

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

// verifyResolvedLink rejects rel if its real-filesystem resolution escapes
// destDir; a dangling or escaping target is rejected identically.
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

// processTarEntry writes one tar entry through root, which scopes filesystem
// ops to destDir via openat2 (kernel-enforced, not textual). Symlinks also get
// a textual pre-check plus an EvalSymlinks post-check, since os.Root validates
// traversal through a link but not its stored target text, and composed
// in-tree links can escape a textual check alone (see verifyResolvedLink).
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
			// Escaped or unreachable: remove it so no non-Root consumer can traverse it, then fail.
			_ = root.Remove(clean)
			return fmt.Errorf("symlink %s: %w", name, err)
		}

	default:
		// Skip unsupported entry types (hardlinks, char/block devices, FIFOs).
	}

	return nil
}

// ExtractTarGz extracts archivePath into destDir, scoping writes through an
// os.Root so zip-slip escapes are rejected at open time. Symlinks additionally
// get a resolution post-check (see processTarEntry) that removes any escaping
// target.
func ExtractTarGz(ctx context.Context, archivePath, destDir string, opts ...ExtractOption) error {
	cfg := &extractConfig{
		logger: logutil.NopLogger,
	}
	for _, o := range opts {
		o(cfg)
	}

	filename := filepath.Base(archivePath)

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
			cfg.logger.Warn("download: could not remove archive", "file", filename, "err", err)
		}
	}

	cfg.logger.Info("download: extracted", "file", filename)

	return nil
}
