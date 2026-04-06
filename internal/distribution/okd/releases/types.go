package releases

import (
	"fmt"
	"time"
)

type ReleaseType int

const (
	ReleaseTypeStable ReleaseType = iota
	ReleaseTypeLatestStable
	ReleaseTypePreview
	ReleaseTypeLatestPreview
	ReleaseTypeLTS
)

type OKDVersion struct {
	Version     string      `json:"version"`
	Tag         string      `json:"tag"`
	ReleaseDate time.Time   `json:"release_date"`
	Stable      bool        `json:"stable"`
	Latest      bool        `json:"latest"`       // Latest in its minor version series
	Type        ReleaseType `json:"release_type"` // Computed release type for display
}

type OKDReleaseSeries struct {
	Major    int
	Minor    int
	Versions []OKDVersion
	Latest   OKDVersion // Latest version in this series
}

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	PublishedAt time.Time `json:"published_at"`
}

type diskCache struct {
	CachedAt time.Time          `json:"cached_at"`
	Series   []OKDReleaseSeries `json:"series"`
}

// Major returns the major version component (e.g. 4 for "4.15.0-okd-0").
// Callers are expected to pass OKDVersion values that have already been
// produced by parseVersionTag, which guarantees a well-formed "N.M" prefix.
// If the Version field was set from unvalidated input, the Sscanf error is
// intentionally discarded and a zero value is returned — that is acceptable
// degraded behavior for a helper used only for sorting and display.
func (v *OKDVersion) Major() int {
	var major int
	_, _ = fmt.Sscanf(v.Version, "%d.", &major)
	return major
}

// Minor returns the minor version component (e.g. 15 for "4.15.0-okd-0").
// See the Major godoc: the input is expected to come from parseVersionTag,
// and unvalidated input degrades to a zero return.
func (v *OKDVersion) Minor() int {
	var major, minor int
	_, _ = fmt.Sscanf(v.Version, "%d.%d", &major, &minor)
	return minor
}

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

func (v *OKDVersion) ShortVersion() string {
	return fmt.Sprintf("%d.%d", v.Major(), v.Minor())
}
