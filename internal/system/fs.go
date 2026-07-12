package system

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// TerraformEnvDir returns the Terraform environment directory
// (<projectRoot>/infrastructure/terraform/environments/<env>) for env. An
// empty env resolves to the environments directory itself. It lives here
// rather than in phase because internal/infrastructure/proxmox must build
// this path without importing phase (see roadmap B1 layering fix).
func TerraformEnvDir(projectRoot, env string) string {
	return filepath.Join(projectRoot, "infrastructure", "terraform", "environments", env)
}

// FileExists reports whether path refers to an existing regular file
// (returns false for directories).
func FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// DirExists reports whether path refers to an existing directory.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// EnsureDir creates path and any missing parents with mode 0o755.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

// EnsureDirForFile creates the parent directory of filePath, including any
// missing intermediate directories, with mode 0o755.
func EnsureDirForFile(filePath string) error {
	dir := filepath.Dir(filePath)
	return EnsureDir(dir)
}

// WriteTempFile creates a temp file matching pattern with mode applied at
// open time (no create-then-chmod window), then calls writeFn with the open
// handle. On any error the file is closed and removed before returning. On
// success, the caller owns cleanup (typically `defer os.Remove(path)`).
//
// Pattern follows os.CreateTemp semantics: if it contains "*", the last "*"
// is replaced by a random numeric suffix; otherwise the suffix is appended.
func WriteTempFile(pattern string, mode os.FileMode, writeFn func(*os.File) error) (string, error) {
	f, err := openTempFile("", pattern, mode)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", pattern, err)
	}
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}
	if err := writeFn(f); err != nil {
		cleanup()
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("close %s: %w", f.Name(), err)
	}
	return f.Name(), nil
}

// openTempFile opens a new exclusive temp file in dir (os.TempDir() if empty)
// with mode set at open time. Replicates os.CreateTemp's "*" substitution and
// collision-retry behaviour without leaving a create-then-chmod window.
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

// CopyFile copies src to dst, preserving the source file's permission bits.
// The destination is opened with the correct mode at creation time via
// CopyFileMode, so there is no window where dst is briefly world-readable
// under a permissive umask. For credential-bearing files (kubeconfig,
// install-config.yaml, private keys), prefer CopyFileMode with an explicit
// 0o600.
func CopyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source file: %w", err)
	}
	return CopyFileMode(src, dst, info.Mode().Perm())
}

// CopyFileMode copies src to dst, creating dst with the given mode applied
// at open time (before any bytes are written). This avoids the race window
// where a file created with a permissive umask is briefly world-readable
// before a follow-up chmod narrows it. Use this for anything sensitive —
// kubeconfigs, credential files, private keys.
//
// Close errors on the destination are surfaced: a failing Close can mask an
// unflushed buffer or fsync problem, and silently discarding it would lose
// a durability signal.
func CopyFileMode(src, dst string, mode os.FileMode) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer func() { _ = sourceFile.Close() }()

	if err := EnsureDirForFile(dst); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	// O_NOFOLLOW rejects a symlink at dst; under the sudo re-exec model the
	// open runs as root, so following a symlink would write to an attacker-chosen path.
	if info, err := os.Lstat(dst); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return &errtypes.AuthError{
				Msg: fmt.Sprintf("write target %q is a symlink; refusing to write", dst),
				Err: os.ErrPermission,
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return &errtypes.AuthError{
			Msg: fmt.Sprintf("failed to lstat write target %q before write", dst),
			Err: err,
		}
	}

	destFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, mode)
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}

	closed := false
	success := false
	defer func() {
		if !closed {
			_ = destFile.Close()
		}
		if !success {
			_ = os.Remove(dst)
		}
	}()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("copy file contents: %w", err)
	}

	if err := destFile.Sync(); err != nil {
		return fmt.Errorf("sync destination file: %w", err)
	}

	// If dst pre-existed with different permissions, O_CREATE won't change
	// them — tighten via the open fd so the chmod cannot follow a symlink.
	if err := destFile.Chmod(mode); err != nil {
		return fmt.Errorf("set file permissions: %w", err)
	}

	if err := destFile.Close(); err != nil {
		return fmt.Errorf("close destination file: %w", err)
	}
	closed = true

	success = true
	return nil
}

// SafeRemove removes path recursively; nil if path does not exist.
func SafeRemove(path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return os.RemoveAll(path)
}

// ExpandPath expands a leading `~/` to the invoking user's home directory.
// Uses InvokingUserHomeDir so that hand-edited config values like
// `~/pull-secret.json` resolve to the shell user's home even after the
// deploy re-execs under sudo (where os.UserHomeDir would return /root).
func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := InvokingUserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// AtomicWrite writes data to path via a temp file in the same directory,
// created with perm at open time, fsyncs it, then renames it into place.
// Concurrent readers see either the old file or the new file, never a
// partial write. The rename is only atomic on the same filesystem — the
// temp file is created next to path to guarantee that.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
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
			Msg: fmt.Sprintf("failed to lstat write target %q before write", path),
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

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write data: %w", err)
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

	// fsync the parent so the rename's directory-entry update is crash-
	// durable. Without this, post-crash the listing can still point at
	// the old name — matters for kubeconfig / .env / install-config.yaml
	// which are consumed immediately after AtomicWrite returns.
	if err := fsyncDir(dir); err != nil {
		return fmt.Errorf("fsync directory: %w", err)
	}

	success = true
	return nil
}

// fsyncDir opens dir read-only and calls fsync so the directory inode
// update from a rename is crash-durable.
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

// AtomicWriteString is a string-typed convenience wrapper around
// AtomicWrite; the fsync + rename invariants are the same.
func AtomicWriteString(path, content string, perm os.FileMode) error {
	return AtomicWrite(path, []byte(content), perm)
}

// IsDirWritable probes by creating and removing a temp file; permission bits
// alone can mislead when ACLs or mount options differ.
func IsDirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, "")
	if err != nil {
		return false
	}
	_ = f.Close()
	_ = os.Remove(f.Name())
	return true
}

// MakeExecutable adds the owner/group/other execute bits to path's existing
// mode. Equivalent to `chmod +x` but without a subprocess.
func MakeExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	return os.Chmod(path, info.Mode().Perm()|0o111)
}

// ChownByName chowns path to the given user:group string. Both parts must
// be present; numeric-only forms are rejected so config typos surface as
// errors rather than silently chowning to UID 0.
func ChownByName(path, ownerSpec string) error {
	userName, groupName, ok := strings.Cut(ownerSpec, ":")
	if !ok || userName == "" || groupName == "" {
		return fmt.Errorf("invalid owner spec %q: want user:group", ownerSpec)
	}
	u, err := user.Lookup(userName)
	if err != nil {
		return fmt.Errorf("lookup user %s: %w", userName, err)
	}
	g, err := user.LookupGroup(groupName)
	if err != nil {
		return fmt.Errorf("lookup group %s: %w", groupName, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return fmt.Errorf("parse uid %q: %w", u.Uid, err)
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return fmt.Errorf("parse gid %q: %w", g.Gid, err)
	}
	return os.Chown(path, uid, gid)
}
