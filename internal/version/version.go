// Package version provides version information for the okdctl CLI.
package version

import (
	"fmt"
	"runtime"
)

// These variables are set at build time using ldflags.
var (
	Version   = "0.1.0"
	GitCommit = "unknown"
	BuildDate = "unknown"
	GoVersion = runtime.Version()
	Platform  = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
)
