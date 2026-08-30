// Package version exposes the okdctl build identity: Version, GitCommit,
// BuildDate, GoVersion, and Platform.
package version

import (
	"fmt"
	"runtime"
)

// Build-time identity, injected via -ldflags by goreleaser and written
// once before main() runs. BackgroundCheck reads Version from a goroutine
// unsynchronized, so any later write is a data race; tests needing a
// different value MUST save/swap/restore via t.Cleanup.
var (
	Version   = "0.1.0"
	GitCommit = "unknown"
	BuildDate = "unknown"
	GoVersion = runtime.Version()
	Platform  = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
)
