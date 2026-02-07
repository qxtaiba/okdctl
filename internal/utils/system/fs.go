// Package system provides system-level utility functions including
// file operations, command execution, HTTP clients, logging, and permissions.
package system

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

// FileExists checks if a file exists and is a regular file (not a directory).
func FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// DirExists checks if a directory exists.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// EnsureDir creates a directory and all parent directories if they don't exist.
// This is equivalent to `mkdir -p`.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// EnsureDirForFile creates the parent directory for a file path.
func EnsureDirForFile(filePath string) error {
	dir := filepath.Dir(filePath)
	return EnsureDir(dir)
}

// CopyFile copies a file from src to dst.
// If dst already exists, it will be overwritten.
// On failure, any partially written destination file is cleaned up.
func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return utils.WrapError("failed to open source file", err)
	}
	defer func() { _ = sourceFile.Close() }()

	// Get source file info for permissions
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return utils.WrapError("failed to stat source file", err)
	}

	// Ensure destination directory exists
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
			_ = os.Remove(dst) // Clean up partial file on failure
		}
	}()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return utils.WrapError("failed to copy file contents", err)
	}

	// Sync to ensure data is flushed to disk
	if err := destFile.Sync(); err != nil {
		return utils.WrapError("failed to sync destination file", err)
	}

	// Preserve permissions
	if err := os.Chmod(dst, sourceInfo.Mode()); err != nil {
		return utils.WrapError("failed to set file permissions", err)
	}

	success = true
	return nil
}

// SafeRemove removes a file or directory if it exists.
// Returns nil if the path doesn't exist.
func SafeRemove(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(path)
}

// ExpandPath expands ~ to home directory in paths.
// Returns the original path unchanged if expansion fails or path doesn't start with ~/.
func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// AtomicWrite writes data to a file atomically using a temp file and rename.
// This ensures the file is never in a partially-written state.
// The temp file is created in the same directory as the target to ensure
// the rename operation is atomic (same filesystem).
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	// Ensure parent directory exists
	if err := EnsureDirForFile(path); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create temp file in same directory for atomic rename
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on any failure
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	// Write data
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write data: %w", err)
	}

	// Sync to ensure data is flushed to disk before rename
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to sync file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Set permissions before rename
	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	success = true
	return nil
}

// AtomicWriteString writes a string to a file atomically.
func AtomicWriteString(path, content string, perm os.FileMode) error {
	return AtomicWrite(path, []byte(content), perm)
}
