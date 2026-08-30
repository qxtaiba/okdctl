//go:build !linux

package doctor

import (
	"fmt"
	"runtime"
)

// fsFreeBytes is unreachable at runtime; the stub exists so package doctor compiles on darwin.
func fsFreeBytes(_ string) (uint64, error) {
	return 0, fmt.Errorf("disk space probe unsupported on %s", runtime.GOOS)
}
