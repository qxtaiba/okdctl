package system

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// FileExists reports whether path refers to an existing regular file
// (returns false for directories).
func FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// EnsureDirForFile creates the parent directory of filePath, including any
// missing intermediate directories, with mode 0o755.
func EnsureDirForFile(filePath string) error {
	return os.MkdirAll(filepath.Dir(filePath), 0o755)
}

// openTempFile opens an exclusive temp file with mode set at open time,
// replicating os.CreateTemp's "*" substitution without a create-then-chmod window.
func openTempFile(dir, pattern string, mode os.FileMode) (*os.File, error) {
	if dir == "" {
		dir = os.TempDir()
	}
	prefix, suffix, _ := strings.Cut(pattern, "*")
	var seed [4]byte
	for range 10000 {
		if _, err := rand.Read(seed[:]); err != nil {
			return nil, fmt.Errorf("read random temp suffix: %w", err)
		}
		name := filepath.Join(dir, prefix+strconv.FormatUint(uint64(binary.BigEndian.Uint32(seed[:])), 10)+suffix)
		f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, mode)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return f, err
	}
	return nil, fmt.Errorf("could not allocate temp file in %s after 10000 tries", dir)
}

// ExpandPath expands a leading `~/` to the user's home directory.
func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// AtomicWrite writes data to path via a temp file synced and renamed into
// place, so concurrent readers see either the old file or the new file,
// never a partial write. The temp file is created next to path (not in
// os.TempDir) since rename is only atomic on the same filesystem.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	return atomicWriteFile(path, perm, func(f *os.File) error {
		if _, err := f.Write(data); err != nil {
			return fmt.Errorf("write data: %w", err)
		}
		return nil
	})
}

func atomicWriteFile(path string, perm os.FileMode, write func(*os.File) error) error {
	if err := EnsureDirForFile(path); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return &errtypes.AuthError{
				Msg: fmt.Sprintf("write target %q is a symlink; refusing to write", path),
				Err: os.ErrPermission,
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return &errtypes.AuthError{
			Msg: fmt.Sprintf("lstat write target %q before write", path),
			Err: err,
		}
	}

	dir := filepath.Dir(path)
	tmpFile, err := openTempFile(dir, ".tmp-*", perm)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := write(tmpFile); err != nil {
		_ = tmpFile.Close()
		return err
	}

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("sync file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	// fsyncs the parent so the rename survives a crash; kubeconfig/.env/
	// install-config.yaml are read immediately after this returns.
	if err := fsyncDir(dir); err != nil {
		return fmt.Errorf("fsync directory: %w", err)
	}

	success = true
	return nil
}

// fsyncDir fsyncs dir so a preceding rename's directory-entry update is crash-durable.
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

// MakeExecutable adds owner/group/other execute bits to path's existing mode
// (chmod +x without a subprocess). The read and write both go through one
// O_NOFOLLOW descriptor, so there's no stat/chmod TOCTOU window and a
// symlink at path is refused rather than followed.
func MakeExecutable(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	return f.Chmod(info.Mode().Perm() | 0o111)
}
