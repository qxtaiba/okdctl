package nodetypes

import (
	"path/filepath"
	"slices"
)

// CoreOSISONamePatterns are the filepath.Match glob shapes of a known-safe
// base CoreOS installer ISO filename. OKD publishes fedora-coreos-*.iso
// through 4.18 and scos-*.iso from 4.19 onward (every 5.x+ major ships
// scos.json exclusively — see provision.streamFileForVersion). provision's local
// ISO auto-detect and hostssh's remote path-safety guard both match
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
