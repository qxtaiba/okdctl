package doctor

import (
	"errors"
	"fmt"
	"syscall"
)

// fsFreeBytes reports the bytes available to unprivileged users on the
// filesystem containing path.
func fsFreeBytes(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, fmt.Errorf("statfs failed: %w", err)
	}
	// Bsize is int64 on linux but a filesystem block size is always
	// positive in practice; the bound check exists to satisfy gosec
	// G115 without a nolint directive.
	if st.Bsize <= 0 {
		return 0, errors.New("statfs returned a non-positive block size")
	}
	return st.Bavail * uint64(st.Bsize), nil
}
