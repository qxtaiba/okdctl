package system

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
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
	return os.MkdirAll(path, 0755)
}

func EnsureDirForFile(filePath string) error {
	dir := filepath.Dir(filePath)
	return EnsureDir(dir)
}

func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return utils.WrapError("failed to open source file", err)
	}
	defer func() { _ = sourceFile.Close() }()

	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return utils.WrapError("failed to stat source file", err)
	}

	if err := EnsureDirForFile(dst); err != nil {
		return utils.WrapError("failed to create destination directory", err)
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return utils.WrapError("failed to create destination file", err)
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
		return utils.WrapError("failed to copy file contents", err)
	}

	if err := destFile.Sync(); err != nil {
		return utils.WrapError("failed to sync destination file", err)
	}

	if err := os.Chmod(dst, sourceInfo.Mode()); err != nil {
		return utils.WrapError("failed to set file permissions", err)
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
