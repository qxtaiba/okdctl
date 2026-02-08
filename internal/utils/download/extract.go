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

type ExtractOptions struct {
	ArchivePath      string
	DestDir          string
	ExpectedChecksum string // SHA256 checksum of the archive (optional)
	StripComponents  int    // Removes leading path components (like tar --strip-components)
	CleanupArchive   bool   // Removes the archive after successful extraction
	Logger           utils.Logger
}

func (o ExtractOptions) logger() utils.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return utils.NoopLogger()
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

func ExtractTarGz(ctx context.Context, opts ExtractOptions) error {
	filename := filepath.Base(opts.ArchivePath)

	if opts.ExpectedChecksum != "" {
		opts.logger().Info(fmt.Sprintf("download: validating sha256 checksum for %s", filename))

		if err := ValidateChecksum(opts.ArchivePath, opts.ExpectedChecksum); err != nil {
			return utils.WrapErrorf(err, "checksum validation failed for %s", filename)
		}

		opts.logger().Info(fmt.Sprintf("download: checksum validated for %s", filename))
	}

	file, err := os.Open(opts.ArchivePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = gzipReader.Close() }()

	tarReader := tar.NewReader(gzipReader)

	if err := os.MkdirAll(opts.DestDir, 0755); err != nil {
		return utils.WrapError("failed to create destination directory", err)
	}

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

	if opts.CleanupArchive {
		if err := os.Remove(opts.ArchivePath); err != nil {
			opts.logger().Warn(fmt.Sprintf("download: failed to cleanup archive %s", filename))
		}
	}

	opts.logger().Info(fmt.Sprintf("download: extracted %s", filename))

	return nil
}
