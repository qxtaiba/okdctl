package system

import (
	"fmt"
	"os"
	"strconv"
)

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
