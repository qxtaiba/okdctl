// Package releases provides dynamic version fetching for Kubernetes distributions.
package releases

import (
	"fmt"
	"time"
)

// ReleaseType categorizes the type of release for display purposes.
type ReleaseType int

const (
	ReleaseTypeStable ReleaseType = iota
	ReleaseTypeLatestStable
	ReleaseTypePreview
	ReleaseTypeLatestPreview
	ReleaseTypeLTS
)

// OKDVersion represents an OKD release version.
type OKDVersion struct {
	Version     string      `json:"version"`
	Tag         string      `json:"tag"`
	ReleaseDate time.Time   `json:"release_date"`
	Stable      bool        `json:"stable"`
	Latest      bool        `json:"latest"`       // Latest in its minor version series
	Type        ReleaseType `json:"release_type"` // Computed release type for display
}

// OKDReleaseSeries represents a series of OKD releases (e.g., 4.21.x).
type OKDReleaseSeries struct {
	Major    int
	Minor    int
	Versions []OKDVersion
	Latest   OKDVersion // Latest version in this series
}

// githubRelease represents a GitHub release API response.
type githubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	PublishedAt time.Time `json:"published_at"`
}

// diskCache represents the on-disk cache structure for OKD versions.
type diskCache struct {
	CachedAt time.Time          `json:"cached_at"`
	Series   []OKDReleaseSeries `json:"series"`
}

// Major returns the major version number.
func (v *OKDVersion) Major() int {
	var major int
	_, _ = fmt.Sscanf(v.Version, "%d.", &major)
	return major
}

// Minor returns the minor version number.
func (v *OKDVersion) Minor() int {
	var major, minor int
	_, _ = fmt.Sscanf(v.Version, "%d.%d", &major, &minor)
	return minor
}

// DisplayName returns a human-readable version name.
func (v *OKDVersion) DisplayName() string {
	switch v.Type {
	case ReleaseTypeLatestStable:
		return fmt.Sprintf("%s (Latest)", v.Version)
	case ReleaseTypeLatestPreview:
		return fmt.Sprintf("%s (Latest Preview)", v.Version)
	case ReleaseTypePreview:
		return fmt.Sprintf("%s (Preview)", v.Version)
	case ReleaseTypeLTS:
		return fmt.Sprintf("%s (LTS)", v.Version)
	default:
		return v.Version
	}
}

// ShortVersion returns the short version (e.g., "4.21").
func (v *OKDVersion) ShortVersion() string {
	return fmt.Sprintf("%d.%d", v.Major(), v.Minor())
}
