//go:build !linux

package doctor

import (
	"errors"
	"runtime"
)

// fsFreeBytes is unreachable at runtime — runDoctor refuses non-linux hosts
// before any probe runs. The stub exists so package doctor compiles on darwin.
func fsFreeBytes(_ string) (uint64, error) {
	return 0, errors.New("disk space probe unsupported on " + runtime.GOOS)
}
