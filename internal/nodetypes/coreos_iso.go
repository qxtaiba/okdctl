package nodetypes

import (
	"path/filepath"
	"slices"
)

// CoreOSISONamePatterns are the known-safe base CoreOS installer ISO filename glob shapes.
// provision's ISO auto-detect and hostssh's path-safety guard both match
// against this list so the two allowlists cannot drift apart.
var CoreOSISONamePatterns = []string{
	"fedora-coreos-*.iso",
	"scos-*.iso",
}

// IsCoreOSISOName reports whether base (a bare filename, no directory
// component) matches one of CoreOSISONamePatterns.
func IsCoreOSISOName(base string) bool {
	return slices.ContainsFunc(CoreOSISONamePatterns, func(pat string) bool {
		ok, _ := filepath.Match(pat, base)
		return ok
	})
}
