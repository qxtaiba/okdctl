// Package version exposes the okdctl build identity: Version, GitCommit,
// BuildDate, GoVersion, and Platform.
package version

import (
	"fmt"
	"runtime"
)

// Build-time identity, injected via -ldflags by goreleaser and written
// once before main() runs.
var (
	Version   = "0.1.0"
	GitCommit = "unknown"
	BuildDate = "unknown"
	GoVersion = runtime.Version()
	Platform  = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
)
