package system

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func DirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func EnsureDirForFile(filePath string) error {
	dir := filepath.Dir(filePath)
	return EnsureDir(dir)
}

// WriteTempFile creates a temp file matching pattern (os.CreateTemp), chmods
// it to mode, then calls writeFn with the open handle. On any error the file
// is closed and removed before returning. On success, the caller owns cleanup
// (typically `defer os.Remove(path)`).
func WriteTempFile(pattern string, mode os.FileMode, writeFn func(*os.File) error) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("failed to create %s: %w", pattern, err)
	}
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}
	if err := f.Chmod(mode); err != nil {
		cleanup()
		return "", fmt.Errorf("failed to chmod %s: %w", f.Name(), err)
	}
	if err := writeFn(f); err != nil {
		cleanup()
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("failed to close %s: %w", f.Name(), err)
	}
	return f.Name(), nil
}

func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() { _ = sourceFile.Close() }()

	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}

	if err := EnsureDirForFile(dst); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}

	success := false
	defer func() {
		_ = destFile.Close()
		if !success {
			_ = os.Remove(dst)
		}
	}()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	if err := destFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync destination file: %w", err)
	}

	if err := os.Chmod(dst, sourceInfo.Mode()); err != nil {
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	success = true
	return nil
}

// CopyFileMode copies src to dst, creating dst with the given mode applied
// at open time (before any bytes are written). This avoids the race window
// where a file created with a permissive umask is briefly world-readable
// before a follow-up chmod narrows it. Use this for anything sensitive —
// kubeconfigs, credential files, private keys.
func CopyFileMode(src, dst string, mode os.FileMode) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() { _ = sourceFile.Close() }()

	if err := EnsureDirForFile(dst); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	destFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}

	success := false
	defer func() {
		_ = destFile.Close()
		if !success {
			_ = os.Remove(dst)
		}
	}()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	if err := destFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync destination file: %w", err)
	}

	// If dst pre-existed with different permissions, O_CREATE won't change
	// them — tighten explicitly so the caller's mode is always honored.
	if err := os.Chmod(dst, mode); err != nil {
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	success = true
	return nil
}

func SafeRemove(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(path)
}

func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	if err := EnsureDirForFile(path); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write data: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to sync file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	success = true
	return nil
}

func AtomicWriteString(path, content string, perm os.FileMode) error {
	return AtomicWrite(path, []byte(content), perm)
}
