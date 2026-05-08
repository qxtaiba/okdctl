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
	"syscall"

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

// WithExtractChecksum sets the expected SHA-256 hex digest for the archive; empty disables verification.
func WithExtractChecksum(sum string) ExtractOption {
	return func(c *extractConfig) { c.expectedChecksum = sum }
}

// WithStripComponents removes n leading path components from archive entries, like tar --strip-components.
func WithStripComponents(n int) ExtractOption {
	return func(c *extractConfig) { c.stripComponents = n }
}

// WithCleanupArchive removes the archive file after successful extraction.
func WithCleanupArchive(v bool) ExtractOption {
	return func(c *extractConfig) { c.cleanupArchive = v }
}

// WithExtractLogger injects a structured logger; nil falls back to logutil.NopLogger.
func WithExtractLogger(l *slog.Logger) ExtractOption {
	return func(c *extractConfig) { c.logger = logutil.OrNop(l) }
}

// verifyResolvedPath checks that path, after resolving symlinks on the real
// filesystem, is still within destDir. The path must already exist.
func verifyResolvedPath(path, cleanDest string) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path %s: %w", path, err)
	}
	if !strings.HasPrefix(filepath.Clean(resolved), cleanDest) {
		return fmt.Errorf("resolves outside destination: %s -> %s", path, resolved)
	}
	return nil
}

func processTarEntry(tarReader *tar.Reader, header *tar.Header, destDir string, stripComponents int) error {
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

	targetPath := filepath.Join(destDir, name) //nolint:gosec // G305: validated below
	cleanDest := filepath.Clean(destDir)

	if !strings.HasPrefix(filepath.Clean(targetPath), cleanDest+string(os.PathSeparator)) && filepath.Clean(targetPath) != cleanDest {
		return fmt.Errorf("archive entry attempts to escape destination: %s", name)
	}

	switch header.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(targetPath, os.FileMode(header.Mode&0o755)); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
		if err := verifyResolvedPath(targetPath, cleanDest); err != nil {
			return fmt.Errorf("directory %s: %w", name, err)
		}

	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}
		// Resolve the parent through the real filesystem to catch writes
		// that traverse a previously-extracted symlink (e.g. link -> /etc
		// followed by link/crontab).
		if err := verifyResolvedPath(filepath.Dir(targetPath), cleanDest); err != nil {
			return fmt.Errorf("file %s: parent %w", name, err)
		}

		// O_NOFOLLOW refuses to open the final component through a symlink,
		// closing the TOCTOU where a previously-extracted symlink would redirect
		// the open onto an attacker-chosen path.
		outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, os.FileMode(header.Mode&0o777))
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}

		if _, err := io.Copy(outFile, tarReader); err != nil {
			_ = outFile.Close()
			return fmt.Errorf("failed to write file: %w", err)
		}
		if err := outFile.Close(); err != nil {
			return fmt.Errorf("failed to close extracted file: %w", err)
		}

	case tar.TypeSymlink:
		linkTarget := header.Linkname
		if filepath.IsAbs(linkTarget) {
			return fmt.Errorf("absolute symlink target not allowed: %s -> %s", name, linkTarget)
		}

		resolvedTarget := filepath.Clean(filepath.Join(filepath.Dir(targetPath), linkTarget)) //nolint:gosec // G305: validated below
		if !strings.HasPrefix(resolvedTarget, cleanDest+string(os.PathSeparator)) && resolvedTarget != cleanDest {
			return fmt.Errorf("symlink target escapes destination: %s -> %s", name, linkTarget)
		}

		if err := os.Symlink(linkTarget, targetPath); err != nil {
			if !os.IsExist(err) {
				return fmt.Errorf("failed to create symlink: %w", err)
			}
			if err := os.Remove(targetPath); err != nil {
				return fmt.Errorf("failed to remove existing symlink %s: %w", name, err)
			}
			if err := os.Symlink(linkTarget, targetPath); err != nil {
				return fmt.Errorf("failed to replace symlink: %w", err)
			}
		}

	default:
		// Skip unsupported entry types (hardlinks, char/block devices, FIFOs).
	}

	return nil
}

// ExtractTarGz extracts archivePath into destDir. Zip-slip and
// symlink-traversal escapes are rejected; path prefixes are rechecked after
// os.EvalSymlinks so writes through previously-extracted links also fail.
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

		if err := ValidateChecksum(archivePath, cfg.expectedChecksum); err != nil {
			return fmt.Errorf("checksum validation failed for %s: %w", filename, err)
		}

		cfg.logger.Info("download: checksum validated", "file", filename)
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive %s: %w", archivePath, err)
	}
	defer func() { _ = file.Close() }()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to decompress archive %s: %w", archivePath, err)
	}
	defer func() { _ = gzipReader.Close() }()

	tarReader := tar.NewReader(gzipReader)

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

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
			return fmt.Errorf("failed to read tar entry from %s: %w", archivePath, err)
		}

		if err := processTarEntry(tarReader, header, destDir, cfg.stripComponents); err != nil {
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
