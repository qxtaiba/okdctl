// Package semver provides semantic versioning utilities.
package semver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version represents a parsed semantic version.
type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Build      string
	Raw        string
}

// versionRegex extracts version components from various formats.
// Handles: v1.2.3, 1.2.3, 1.2.3-beta.1, 1.2.3+build, and more.
var versionRegex = regexp.MustCompile(`v?(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:-([a-zA-Z0-9.-]+))?(?:\+([a-zA-Z0-9.-]+))?`)

// Parse parses a version string into a Version struct.
// It's lenient and handles various formats like "v1.2.3", "1.2", "Terraform v1.5.7".
func Parse(s string) (*Version, error) {
	if s == "" {
		return nil, fmt.Errorf("empty version string")
	}

	matches := versionRegex.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("invalid version format: %s", s)
	}

	v := &Version{Raw: s}

	// Major is required
	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil, fmt.Errorf("invalid major version: %s", matches[1])
	}
	v.Major = major

	// Minor is optional
	if matches[2] != "" {
		minor, err := strconv.Atoi(matches[2])
		if err != nil {
			return nil, fmt.Errorf("invalid minor version: %s", matches[2])
		}
		v.Minor = minor
	}

	// Patch is optional
	if matches[3] != "" {
		patch, err := strconv.Atoi(matches[3])
		if err != nil {
			return nil, fmt.Errorf("invalid patch version: %s", matches[3])
		}
		v.Patch = patch
	}

	// Prerelease is optional
	if matches[4] != "" {
		v.Prerelease = matches[4]
	}

	// Build metadata is optional
	if matches[5] != "" {
		v.Build = matches[5]
	}

	return v, nil
}

// Compare compares two versions.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
func Compare(a, b *Version) int {
	// Compare major
	if a.Major != b.Major {
		if a.Major < b.Major {
			return -1
		}
		return 1
	}

	// Compare minor
	if a.Minor != b.Minor {
		if a.Minor < b.Minor {
			return -1
		}
		return 1
	}

	// Compare patch
	if a.Patch != b.Patch {
		if a.Patch < b.Patch {
			return -1
		}
		return 1
	}

	// Compare prerelease (empty > non-empty, e.g., 1.0.0 > 1.0.0-beta)
	if a.Prerelease == "" && b.Prerelease != "" {
		return 1
	}
	if a.Prerelease != "" && b.Prerelease == "" {
		return -1
	}
	if a.Prerelease != b.Prerelease {
		return strings.Compare(a.Prerelease, b.Prerelease)
	}

	return 0
}

// AtLeast returns true if v is greater than or equal to min.
func (v *Version) AtLeast(min *Version) bool {
	return Compare(v, min) >= 0
}

// AtLeastString returns true if v is greater than or equal to minVersion string.
func (v *Version) AtLeastString(minVersion string) bool {
	min, err := Parse(minVersion)
	if err != nil {
		return false
	}
	return v.AtLeast(min)
}

// String returns the version as a string.
func (v *Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		s += "-" + v.Prerelease
	}
	if v.Build != "" {
		s += "+" + v.Build
	}
	return s
}

// ExtractVersion extracts a version from a string containing version output.
// Useful for parsing "terraform v1.5.7 on darwin_arm64" or similar.
func ExtractVersion(output string) (*Version, error) {
	// Try each line
	for _, line := range strings.Split(output, "\n") {
		if v, err := Parse(line); err == nil {
			return v, nil
		}
	}

	// Try the whole string as a last resort
	return Parse(output)
}
