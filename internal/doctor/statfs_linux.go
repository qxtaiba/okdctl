package doctor

import (
	"errors"
	"fmt"
	"syscall"
)

// fsFreeBytes reports bytes available to unprivileged users on path's filesystem.
func fsFreeBytes(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, fmt.Errorf("statfs failed: %w", err)
	}
	// Bsize is int64 but always positive in practice; this check satisfies
	// gosec G115 without a nolint directive.
	if st.Bsize <= 0 {
		return 0, errors.New("statfs returned a non-positive block size")
	}
	return st.Bavail * uint64(st.Bsize), nil
}
