// Package version provides version information for the openshitctl CLI.
package version

import (
	"fmt"
	"runtime"
)

// These variables are set at build time using ldflags.
var (
	// Version is the semantic version of the CLI.
	Version = "0.1.0"

	// GitCommit is the git commit hash.
	GitCommit = "unknown"

	// BuildDate is the date the binary was built.
	BuildDate = "unknown"

	// GoVersion is the version of Go used to build.
	GoVersion = runtime.Version()

	// Platform is the OS/architecture.
	Platform = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
)
