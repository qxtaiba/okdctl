package system

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
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
