// Package runlock provides a process-level advisory lock, flock-based so
// the kernel releases it on fd close (including SIGKILL), avoiding the
// PID-reuse race of pid-file schemes. On NFS pre-v4, flock is advisory
// only and may not enforce exclusion across hosts.
package runlock

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

const lockFile = ".okdctl.lock"

// Lock holds the open file descriptor whose flock protects the project root.
// Call Release when the protected operation completes.
type Lock struct {
	f *os.File
}

// Acquire opens <projectRoot>/.okdctl.lock, takes an exclusive
// non-blocking flock, and returns a *Lock, or a *errtypes.ConfigError
// naming the holder on conflict. Warn records route through
// slog.Default(); callers bypassing cli.Execute must install
// logutil.RedactHandler first.
func Acquire(projectRoot, verb string) (*Lock, error) {
	path := filepath.Join(projectRoot, lockFile)

	// Refuse a symlink at the lock path (O_NOFOLLOW closes the lstat->open
	// TOCTOU race) — Acquire runs as root under sudo re-exec, so a pre-sudo
	// attacker could otherwise redirect the root-owned write via a symlink.
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, &errtypes.ConfigError{
				Msg: fmt.Sprintf("project lock %s is a symlink; refusing to follow", path),
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, &errtypes.ConfigError{
			Msg: fmt.Sprintf("runlock: lstat %s", path),
			Err: err,
		}
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, &errtypes.ConfigError{
			Msg: fmt.Sprintf("runlock: open %s", path),
			Err: err,
		}
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		body, _ := os.ReadFile(path)
		_ = f.Close()
		holder := string(body)
		if holder == "" {
			holder = "(unknown)"
		}
		localHost, hostErr := os.Hostname()
		if hostErr != nil {
			localHost = "unknown"
		}
		msg := fmt.Sprintf("another okdctl process holds the project lock: %s", holder)
		if hint := crossHostHint(holder, localHost); hint != "" {
			msg += "; " + hint
		}
		return nil, &errtypes.ConfigError{Msg: msg, Err: err}
	}

	host, hostErr := os.Hostname()
	if hostErr != nil {
		slog.Warn("runlock: os.Hostname failed, using unknown", "err", hostErr)
		host = "unknown"
	}
	// Truncate then write diagnostics; failures are best-effort.
	_ = f.Truncate(0)
	_, _ = fmt.Fprintf(f, "PID=%d VERB=%s TIME=%s HOST=%s\n", os.Getpid(), verb, time.Now().UTC().Format(time.RFC3339), host)

	if err := chownToSudoInvoker(path); err != nil {
		slog.Warn("runlock: chown lockfile to invoking user failed", "path", path, "err", err)
	}

	return &Lock{f: f}, nil
}

// chownToSudoInvoker hands the root-created lockfile back to the invoking
// user so --dry-run (non-root) can reopen it; the euid guard skips this
// when SUDO_UID/SUDO_GID leak into a non-root env, which would EPERM.
func chownToSudoInvoker(path string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	return system.ChownToInvokingUser(path)
}

// crossHostHint flags a HOST= mismatch as a likely NFSv3 cross-host stale
// lock, where flock doesn't propagate.
func crossHostHint(body, localHost string) string {
	for field := range strings.FieldsSeq(body) {
		val, ok := strings.CutPrefix(field, "HOST=")
		if !ok {
			continue
		}
		if val != "" && val != localHost {
			return "lock holder is on a different host (NFS-detected). " +
				"On NFSv3 flock is advisory across hosts — verify with 'fuser .okdctl.lock' before deleting"
		}
		return ""
	}
	return ""
}

// Release truncates the diagnostics and closes the fd, surrendering the
// flock; the lockfile is left in place since removing it would reintroduce
// the flock inode race across concurrent Acquire calls. No-op on a nil
// receiver or zero-value Lock.
func (l *Lock) Release() {
	if l == nil || l.f == nil {
		return
	}
	_ = l.f.Truncate(0)
	_ = l.f.Close()
}
