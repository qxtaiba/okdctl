// Package version provides version information for the openshiftctl CLI.
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

// Info contains version information.
type Info struct {
	Version   string `json:"version"`
	GitCommit string `json:"gitCommit"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

// Get returns the version information.
func Get() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		GoVersion: GoVersion,
		Platform:  Platform,
	}
}

