package system

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// invokingUser returns root under direct-root invocation (SUDO_USER unset);
// callers needing the non-root identity must guard with os.Geteuid() == 0.
func invokingUser() (*user.User, error) {
	if name := os.Getenv("SUDO_USER"); name != "" {
		if u, err := user.Lookup(name); err == nil {
			return u, nil
		}
	}
	return user.Current()
}

// InvokingUserHomeDir returns the invoking user's home directory instead of
// os.UserHomeDir(), which returns /root under sudo's env reset. Under direct
// root invocation (no SUDO_USER) this also returns /root; guard with
// os.Geteuid() == 0 if that matters to the caller.
func InvokingUserHomeDir() (string, error) {
	u, err := invokingUser()
	if err != nil {
		return "", err
	}
	return u.HomeDir, nil
}

type sudoIDs struct {
	uid, gid int
}

// invokingUserIDs returns SUDO_UID/SUDO_GID, or nil if not running under sudo
// (chown then unnecessary).
func invokingUserIDs() (*sudoIDs, error) {
	uidStr := os.Getenv("SUDO_UID")
	gidStr := os.Getenv("SUDO_GID")
	if uidStr == "" || gidStr == "" {
		return nil, nil
	}
	uid, err := strconv.Atoi(uidStr)
	if err != nil {
		return nil, fmt.Errorf("invalid SUDO_UID %q: %w", uidStr, err)
	}
	gid, err := strconv.Atoi(gidStr)
	if err != nil {
		return nil, fmt.Errorf("invalid SUDO_GID %q: %w", gidStr, err)
	}
	return &sudoIDs{uid: uid, gid: gid}, nil
}

// ChownToInvokingUser chowns path to SUDO_UID:SUDO_GID via Lchown, so a
// symlink planted between write and chown is chowned itself rather than
// redirected. No-ops when not running under sudo.
func ChownToInvokingUser(path string) error {
	ids, err := invokingUserIDs()
	if err != nil || ids == nil {
		return err
	}
	return os.Lchown(path, ids.uid, ids.gid)
}

// ChownFileToInvokingUser chowns the open file's descriptor to
// SUDO_UID:SUDO_GID, so a path swapped for a symlink after open can't
// redirect the chown. No-ops when not running under sudo.
func ChownFileToInvokingUser(f *os.File) error {
	ids, err := invokingUserIDs()
	if err != nil || ids == nil {
		return err
	}
	return f.Chown(ids.uid, ids.gid)
}

// statFn is the os.Stat seam WriteAsInvokingUser uses; tests override it to
// drive parentExisted without filesystem setup.
var statFn = os.Stat

// WriteAsInvokingUser atomically writes data to path with mode, then chowns
// the file (and, if AtomicWrite created it, the parent directory) to the
// invoking user. Credential-bearing files must use mode 0o600 — a looser
// mode leaks secret content to other local users.
func WriteAsInvokingUser(path string, data []byte, mode os.FileMode) error {
	parentDir := filepath.Dir(path)
	parentExisted := true
	if _, err := statFn(parentDir); errors.Is(err, os.ErrNotExist) {
		parentExisted = false
	}
	if err := AtomicWrite(path, data, mode); err != nil {
		return err
	}
	if err := ChownToInvokingUser(path); err != nil {
		return err
	}
	if !parentExisted {
		return ChownToInvokingUser(parentDir)
	}
	return nil
}

// canonicalizePath resolves symlinks (so /tmp vs /private/tmp on macOS doesn't
// split the allowlist), falling back to the input on failure.
func canonicalizePath(p string) string {
	if p == "" {
		return p
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return p
}

// isAllowedChownRoot guards recursive chown against paths like /etc; allowed
// are okdctl's own WorkDir/infrastructure trees, subpaths of homeDir, and
// subpaths of tmpDir.
func isAllowedChownRoot(absPath, homeDir, tmpDir string) bool {
	base := filepath.Base(absPath)
	if base == workspace.WorkDirName || base == "infrastructure" {
		return true
	}
	hasPrefix := func(prefix string) bool {
		if prefix == "" {
			return false
		}
		if absPath == prefix {
			return true
		}
		return strings.HasPrefix(absPath, prefix+string(filepath.Separator))
	}
	return hasPrefix(homeDir) || hasPrefix(tmpDir)
}

// ChownTreeToInvokingUser recursively chowns root and its descendants to the
// invoking user, continuing past per-entry errors rather than aborting the
// whole walk; a no-op when not running under sudo. The walk runs through
// os.Root, so directory-component symlinks can't redirect a chown outside
// the trust root.
func ChownTreeToInvokingUser(root string) error {
	ids, err := invokingUserIDs()
	if err != nil || ids == nil {
		return err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve path %s: %w", root, err)
	}
	homeDir, _ := InvokingUserHomeDir()
	tmpDir := os.TempDir()
	if !isAllowedChownRoot(canonicalizePath(absRoot), canonicalizePath(homeDir), canonicalizePath(tmpDir)) {
		return &errtypes.AuthError{
			Msg:  "chown tree refused: path is not in an okdctl-managed subtree",
			Path: absRoot,
		}
	}
	osRoot, openErr := os.OpenRoot(root)
	if openErr != nil {
		return fmt.Errorf("open root %s: %w", root, openErr)
	}
	defer func() { _ = osRoot.Close() }()
	var errs []error
	if walkErr := fs.WalkDir(osRoot.FS(), ".", func(path string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			errs = append(errs, walkErr)
			return nil
		}
		if chownErr := osRoot.Lchown(path, ids.uid, ids.gid); chownErr != nil {
			errs = append(errs, fmt.Errorf("chown %s: %w", path, chownErr))
		}
		return nil
	}); walkErr != nil {
		errs = append(errs, walkErr)
	}
	return errors.Join(errs...)
}

// HasPasswordlessSudo returns nil if `sudo -n true` succeeds, used as an
// advisory pre-flight before a sudo prompt might appear.
func HasPasswordlessSudo(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "sudo", "-n", "true")
	// Minimal PATH-only env: the allowlisted parent env deliberately keeps
	// secret-keyed entries for okdctl's own re-exec, and this child has no use for them.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	err := cmd.Run()
	// Mirrors executor.NewExitError: surfaces ctx.Err() on a ctx-killed sudo so
	// errors.Is(context.Canceled/DeadlineExceeded) sees cancellation, not an opaque exec.ExitError.
	if ctxErr := ctx.Err(); err != nil && ctxErr != nil {
		return ctxErr
	}
	return err
}
