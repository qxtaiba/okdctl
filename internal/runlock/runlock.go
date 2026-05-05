// Package runlock provides a process-level advisory lock for okdctl
// operations that mutate shared project state. The lock is flock-based
// so the kernel releases it automatically on fd close — including SIGKILL —
// eliminating the PID-reuse race of pure pid-file schemes. On NFS pre-v4
// flock is advisory only and may not enforce mutual exclusion across hosts.
package runlock

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

const lockFile = ".okdctl.lock"

// Lock holds the open file descriptor whose flock protects the project root.
// Call Release when the protected operation completes.
type Lock struct {
	f *os.File
}

// Acquire opens <projectRoot>/.okdctl.lock and takes an exclusive
// non-blocking flock. On success it writes human-readable diagnostics
// into the file and returns a *Lock. On conflict it reads the file body
// and returns a *errtypes.ConfigError naming the holder.
func Acquire(projectRoot, verb string) (*Lock, error) {
	path := filepath.Join(projectRoot, lockFile)

	// Refuse a symlink at the lock path and open with O_NOFOLLOW so a symlink
	// planted between lstat and open still loses the race. Needed because
	// Acquire runs as root under the deploy/destroy sudo re-exec model and a
	// pre-sudo attacker could otherwise redirect the root-owned write via a
	// planted symlink.
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, &errtypes.ConfigError{
				Msg: fmt.Sprintf("project lock %s is a symlink; refusing to follow", path),
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("runlock: lstat %s: %w", path, err)
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("runlock: open %s: %w", path, err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		body, _ := os.ReadFile(path)
		_ = f.Close()
		holder := string(body)
		if holder == "" {
			holder = "(unknown)"
		}
		return nil, &errtypes.ConfigError{
			Msg: fmt.Sprintf("another okdctl process holds the project lock: %s", holder),
		}
	}

	host, hostErr := os.Hostname()
	if hostErr != nil {
		slog.Warn("runlock: os.Hostname failed, using unknown", "err", hostErr)
		host = "unknown"
	}
	// Truncate then write diagnostics; failures are best-effort.
	_ = f.Truncate(0)
	_, _ = fmt.Fprintf(f, "PID=%d VERB=%s TIME=%s HOST=%s\n", os.Getpid(), verb, time.Now().UTC().Format(time.RFC3339), host)

	return &Lock{f: f}, nil
}

// Release closes the fd (which releases the flock) and removes the lock file.
// Release is a no-op on a nil receiver or a zero-value Lock.
func (l *Lock) Release() {
	if l == nil || l.f == nil {
		return
	}
	path := l.f.Name()
	_ = l.f.Close()
	_ = os.Remove(path)
}
