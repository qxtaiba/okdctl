// Package runlock provides a process-level advisory lock for okdctl
// operations that mutate shared project state. The lock is flock-based
// so the kernel releases it automatically on fd close — including SIGKILL —
// eliminating the PID-reuse race of pure pid-file schemes.
package runlock

import (
	"fmt"
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

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
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

	// Truncate then write diagnostics; failures are best-effort.
	_ = f.Truncate(0)
	_, _ = fmt.Fprintf(f, "PID=%d VERB=%s TIME=%s\n", os.Getpid(), verb, time.Now().UTC().Format(time.RFC3339))

	return &Lock{f: f}, nil
}

// Release closes the fd (which releases the flock) and removes the lock file.
func (l *Lock) Release() {
	path := l.f.Name()
	_ = l.f.Close()
	_ = os.Remove(path)
}
