package releases

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ReleaseType classifies an OKD release for display. Values drive DisplayName
// suffixes ("(Latest)", "(Preview)", etc.) surfaced by the releases CLI.
type ReleaseType int

const (
	// ReleaseTypeStable is a shipped-stable release.
	ReleaseTypeStable ReleaseType = iota
	// ReleaseTypeLatestStable is the newest stable in its minor series.
	ReleaseTypeLatestStable
	// ReleaseTypePreview is a pre-release (preview) build.
	ReleaseTypePreview
	// ReleaseTypeLatestPreview is the newest preview in its minor series.
	ReleaseTypeLatestPreview
	// ReleaseTypeLTS is a long-term-support release.
	ReleaseTypeLTS
)

// OKDVersion is one OKD release entry fetched from the release catalog.
// JSON field names are part of the `okdctl releases list --output=json`
// contract; do not rename a field without updating downstream consumers.
type OKDVersion struct {
	Version     string      `json:"version"`
	Tag         string      `json:"tag"`
	ReleaseDate time.Time   `json:"release_date"`
	Stable      bool        `json:"stable"`
	Latest      bool        `json:"latest"`       // Latest in its minor version series
	Type        ReleaseType `json:"release_type"` // Computed release type for display
}

// OKDReleaseSeries groups releases by major.minor with the newest member
// cached as Latest for O(1) lookup.
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

// String returns the canonical human-readable label for the release type.
func (t ReleaseType) String() string {
	switch t {
	case ReleaseTypeStable:
		return "stable"
	case ReleaseTypeLatestStable:
		return "latest-stable"
	case ReleaseTypePreview:
		return "preview"
	case ReleaseTypeLatestPreview:
		return "latest-preview"
	case ReleaseTypeLTS:
		return "lts"
	default:
		return "unknown"
	}
}

// MarshalJSON encodes ReleaseType as its string label so OKDVersion serialises
// with "release_type": "stable" rather than a raw integer.
func (t ReleaseType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON decodes the string label back into the typed constant.
// Required for round-trip correctness of the on-disk release cache.
func (t *ReleaseType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "stable":
		*t = ReleaseTypeStable
	case "latest-stable":
		*t = ReleaseTypeLatestStable
	case "preview":
		*t = ReleaseTypePreview
	case "latest-preview":
		*t = ReleaseTypeLatestPreview
	case "lts":
		*t = ReleaseTypeLTS
	default:
		return fmt.Errorf("unknown release type %q", s)
	}
	return nil
}

// Major returns the major version component, or 0 for unparsable input.
func (v *OKDVersion) Major() int {
	before, _, _ := strings.Cut(v.Version, ".")
	n, _ := strconv.Atoi(before)
	return n
}

// Minor returns the minor version component, or 0 for unparsable input.
func (v *OKDVersion) Minor() int {
	_, after, ok := strings.Cut(v.Version, ".")
	if !ok {
		return 0
	}
	before, _, _ := strings.Cut(after, ".")
	n, _ := strconv.Atoi(before)
	return n
}

// DisplayName returns the version string with a release-type suffix
// ("4.15.1 (Latest)"), used by the releases list CLI renderer.
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

// ShortVersion returns the "major.minor" form (e.g. "4.15") of the version,
// or "0.0" when Version is unparsable.
func (v *OKDVersion) ShortVersion() string {
	return fmt.Sprintf("%d.%d", v.Major(), v.Minor())
}
