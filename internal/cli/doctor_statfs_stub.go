//go:build !linux

package cli

import (
	"errors"
	"runtime"
)

// fsFreeBytes is unreachable at runtime — runDoctor refuses non-linux hosts
// before any probe runs. The stub exists so package cli compiles on darwin.
func fsFreeBytes(_ string) (uint64, error) {
	return 0, errors.New("disk space probe unsupported on " + runtime.GOOS)
}
