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

// ExtractOptions configures an ExtractTarGz call.
type ExtractOptions struct {
	ArchivePath      string
	DestDir          string
	ExpectedChecksum string // SHA256 checksum of the archive (optional)
	StripComponents  int    // Removes leading path components (like tar --strip-components)
	CleanupArchive   bool   // Removes the archive after successful extraction
	Logger           *slog.Logger
}

func (o ExtractOptions) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return logutil.NopLogger
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
		if err := os.MkdirAll(targetPath, os.FileMode(header.Mode&0o777)); err != nil {
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

		outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode&0o777))
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

// ExtractTarGz extracts opts.ArchivePath into opts.DestDir. Zip-slip and
// symlink-traversal escapes are rejected; path prefixes are rechecked after
// os.EvalSymlinks so writes through previously-extracted links also fail.
func ExtractTarGz(ctx context.Context, opts ExtractOptions) error {
	filename := filepath.Base(opts.ArchivePath)

	if opts.ExpectedChecksum != "" {
		opts.logger().Info(fmt.Sprintf("download: validating sha256 checksum for %s", filename))

		if err := ValidateChecksum(opts.ArchivePath, opts.ExpectedChecksum); err != nil {
			return fmt.Errorf("checksum validation failed for %s: %w", filename, err)
		}

		opts.logger().Info(fmt.Sprintf("download: checksum validated for %s", filename))
	}

	file, err := os.Open(opts.ArchivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive %s: %w", opts.ArchivePath, err)
	}
	defer func() { _ = file.Close() }()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to decompress archive %s: %w", opts.ArchivePath, err)
	}
	defer func() { _ = gzipReader.Close() }()

	tarReader := tar.NewReader(gzipReader)

	if err := os.MkdirAll(opts.DestDir, 0o755); err != nil {
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
			return fmt.Errorf("failed to read tar entry from %s: %w", opts.ArchivePath, err)
		}

		if err := processTarEntry(tarReader, header, opts.DestDir, opts.StripComponents); err != nil {
			return err
		}
	}

	if opts.CleanupArchive {
		if err := os.Remove(opts.ArchivePath); err != nil {
			opts.logger().Warn(fmt.Sprintf("download: failed to cleanup archive %s", filename))
		}
	}

	opts.logger().Info(fmt.Sprintf("download: extracted %s", filename))

	return nil
}
