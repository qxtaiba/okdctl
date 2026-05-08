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
)

// InvokingUser returns the user who invoked the command. When the process
// was re-exec'd under sudo (as okdctl deploy / destroy / cleanup /
// update-ingress do), SUDO_USER identifies the original user. Without it,
// the current user is returned. If SUDO_USER names a user that no longer
// exists (deleted mid-run), we fall back to the current user rather than
// failing the deploy — a late-stage chown-back is best-effort.
func InvokingUser() (*user.User, error) {
	if name := os.Getenv("SUDO_USER"); name != "" {
		if u, err := user.Lookup(name); err == nil {
			return u, nil
		}
	}
	return user.Current()
}

// InvokingUserHomeDir returns the home directory of the invoking user. Use
// this instead of os.UserHomeDir() at sites that write artifacts the user
// must read back (kubeconfig, releases cache, .bashrc). os.UserHomeDir()
// returns /root under sudo's default env reset, which would land files in
// the wrong place.
func InvokingUserHomeDir() (string, error) {
	u, err := InvokingUser()
	if err != nil {
		return "", err
	}
	return u.HomeDir, nil
}

type sudoIDs struct {
	uid, gid int
}

// invokingUserIDs returns the SUDO_UID/SUDO_GID pair, or nil if the process
// was not re-exec'd under sudo (chown is then unnecessary).
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

// ChownToInvokingUser chowns path to SUDO_UID:SUDO_GID. Silently no-ops when
// the process was not re-exec'd under sudo — in that case the caller is
// already the invoking user and the file is already owned correctly.
func ChownToInvokingUser(path string) error {
	ids, err := invokingUserIDs()
	if err != nil || ids == nil {
		return err
	}
	return os.Chown(path, ids.uid, ids.gid)
}

// statFn is the os.Stat seam used by WriteAsInvokingUser; tests override it
// to drive the parentExisted flag without filesystem setup.
var statFn = os.Stat

// WriteAsInvokingUser atomically writes data to path with mode, then chowns
// the file to the invoking user (if running under sudo). If AtomicWrite had
// to create the immediate parent directory (i.e. it didn't exist before),
// that directory is also chowned — otherwise we leave pre-existing
// directory ownership untouched to avoid silently chowning a directory the
// user explicitly created with a different owner.
func WriteAsInvokingUser(path string, data []byte, mode os.FileMode) error {
	parentDir := filepath.Dir(path)
	parentExisted := true
	if _, err := statFn(parentDir); os.IsNotExist(err) {
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

// canonicalizePath resolves symlinks to a stable form so /tmp vs
// /private/tmp (macOS) does not split the allowlist comparison. Falls back
// to the input on EvalSymlinks failure (e.g. path not yet on disk).
func canonicalizePath(p string) string {
	if p == "" {
		return p
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return p
}

// isAllowedChownRoot reports whether absPath is a permitted target for a
// recursive chown. Guard against caller passing /etc or similar — chowntree
// running as root would corrupt system ownership. Allowed: paths whose Base
// is "okd-install" or "infrastructure" (the only trees okdctl creates as
// root), any subpath of homeDir (kubeconfig install + .okdctl cache), and
// any subpath of tmpDir (test/ephemeral flows).
func isAllowedChownRoot(absPath, homeDir, tmpDir string) bool {
	base := filepath.Base(absPath)
	if base == "okd-install" || base == "infrastructure" {
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

// ChownTreeToInvokingUser recursively chowns root and all descendants to the
// invoking user. No-op if the process was not re-exec'd under sudo. Errors
// on individual entries are collected; the walk does not abort so a single
// unreadable entry doesn't leave the rest of the tree root-owned.
//
// The walk runs through os.Root so directory-component symlinks cannot
// redirect any Lchown outside the trust root.
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

// HasPasswordlessSudo returns nil if `sudo -n true` succeeds. Callers use
// this as an advisory pre-flight to warn the user that the next sudo
// invocation may prompt for a password. Under the re-exec model this is
// only called by doctor; operational paths never call it.
func HasPasswordlessSudo(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "sudo", "-n", "true")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}
