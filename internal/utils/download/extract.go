package download

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

// ExtractOptions configures archive extraction.
type ExtractOptions struct {
	// ArchivePath is the path to the archive file.
	ArchivePath string

	// DestDir is the destination directory for extraction.
	DestDir string

	// ExpectedChecksum is the expected SHA256 checksum of the archive (optional).
	ExpectedChecksum string

	// StripComponents removes leading path components (like tar --strip-components).
	StripComponents int

	// CleanupArchive removes the archive after successful extraction.
	CleanupArchive bool
}

// processTarEntry handles a single tar entry (dir, file, or symlink).
func processTarEntry(tarReader *tar.Reader, header *tar.Header, destDir string, stripComponents int) error {
	// Apply strip components
	name := header.Name
	if stripComponents > 0 {
		parts := strings.Split(name, "/")
		if len(parts) <= stripComponents {
			return nil // Skip this entry
		}
		name = strings.Join(parts[stripComponents:], "/")
	}

	if name == "" {
		return nil
	}

	targetPath := filepath.Join(destDir, name)

	// Security check: ensure path doesn't escape destination
	if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(destDir)) {
		return fmt.Errorf("archive entry attempts to escape destination: %s", name)
	}

	switch header.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
			return utils.WrapError("failed to create directory", err)
		}

	case tar.TypeReg:
		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return utils.WrapError("failed to create parent directory", err)
		}

		outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
		if err != nil {
			return utils.WrapError("failed to create file", err)
		}

		if _, err := io.Copy(outFile, tarReader); err != nil {
			_ = outFile.Close()
			return utils.WrapError("failed to write file", err)
		}
		_ = outFile.Close()

	case tar.TypeSymlink:
		// Validate symlink target doesn't escape destination
		linkTarget := header.Linkname
		if filepath.IsAbs(linkTarget) {
			return fmt.Errorf("absolute symlink target not allowed: %s -> %s", name, linkTarget)
		}

		// Resolve the symlink target relative to the target path's directory
		resolvedTarget := filepath.Join(filepath.Dir(targetPath), linkTarget)
		resolvedTarget = filepath.Clean(resolvedTarget)

		if !strings.HasPrefix(resolvedTarget, filepath.Clean(destDir)) {
			return fmt.Errorf("symlink target escapes destination: %s -> %s", name, linkTarget)
		}

		if err := os.Symlink(linkTarget, targetPath); err != nil {
			// Ignore symlink errors on Windows
			if !os.IsExist(err) {
				return utils.WrapError("failed to create symlink", err)
			}
		}
	}

	return nil
}

// ExtractTarGz extracts a .tar.gz archive to a destination directory.
func ExtractTarGz(ctx context.Context, opts ExtractOptions) error {
	filename := filepath.Base(opts.ArchivePath)

	// Validate checksum if provided
	if opts.ExpectedChecksum != "" {
		utils.GetLogger().Info(fmt.Sprintf("download: validating sha256 checksum for %s", filename))

		if err := ValidateChecksum(opts.ArchivePath, opts.ExpectedChecksum); err != nil {
			return utils.WrapErrorf(err, "checksum validation failed for %s", filename)
		}

		utils.GetLogger().Info(fmt.Sprintf("download: checksum validated for %s", filename))
	}

	// Open the archive
	file, err := os.Open(opts.ArchivePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	// Create gzip reader
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = gzipReader.Close() }()

	// Create tar reader
	tarReader := tar.NewReader(gzipReader)

	// Ensure destination directory exists
	if err := os.MkdirAll(opts.DestDir, 0755); err != nil {
		return utils.WrapError("failed to create destination directory", err)
	}

	// Extract files
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if err := processTarEntry(tarReader, header, opts.DestDir, opts.StripComponents); err != nil {
			return err
		}
	}

	// Cleanup archive if requested
	if opts.CleanupArchive {
		if err := os.Remove(opts.ArchivePath); err != nil {
			utils.GetLogger().Warn(fmt.Sprintf("download: failed to cleanup archive %s", filename))
		}
	}

	utils.GetLogger().Info(fmt.Sprintf("download: extracted %s", filename))

	return nil
}
