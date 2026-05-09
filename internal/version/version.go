// Package version exposes the okdctl build identity: Version, GitCommit,
// BuildDate, GoVersion, and Platform. All values are -ldflags-injected
// before main and read race-free at runtime; tests must save/restore via
// t.Cleanup to avoid leaking mutations across boundaries.
package version

import (
	"fmt"
	"runtime"
)

// Build-time identity variables injected via -ldflags by goreleaser. They
// are written exactly once before main() runs and must not be written by
// production code afterwards: BackgroundCheck (updatecheck.go) reads
// Version from a goroutine without synchronisation, so a concurrent write
// is a data race. Tests that need a controlled value MUST save the
// original, swap, and restore in a t.Cleanup callback so no mutation
// leaks across test boundaries.
var (
	Version   = "0.1.0"
	GitCommit = "unknown"
	BuildDate = "unknown"
	GoVersion = runtime.Version()
	Platform  = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
)
