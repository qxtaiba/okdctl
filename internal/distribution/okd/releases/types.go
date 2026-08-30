package releases

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ReleaseType classifies an OKD release for display (drives DisplayName's
// "(Latest)"/"(Preview)" suffixes).
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

// OKDVersion is one release entry from the catalog. JSON field names are the
// `releases list --output=json` wire contract — do not rename without updating consumers.
type OKDVersion struct {
	Version     string      `json:"version"`
	Tag         string      `json:"tag"`
	ReleaseDate time.Time   `json:"release_date"`
	Stable      bool        `json:"stable"`
	Latest      bool        `json:"latest"` // within its minor series
	Type        ReleaseType `json:"release_type"`
}

// OKDReleaseSeries groups releases by major.minor, caching the newest as Latest for O(1) lookup.
type OKDReleaseSeries struct {
	Major    int
	Minor    int
	Versions []OKDVersion
	Latest   OKDVersion
}

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	Draft       bool      `json:"draft"`
	PublishedAt time.Time `json:"published_at"`
}

// diskCacheSchema versions the cache JSON: json.Unmarshal tolerates shape
// drift silently, so a schema mismatch is discarded outright; bump on any shape change.
const diskCacheSchema = 1

type diskCache struct {
	Schema   int                `json:"schema"`
	CachedAt time.Time          `json:"cached_at"`
	Series   []OKDReleaseSeries `json:"series"`
}

// releaseTypeLabels is the single wire-label source for String and
// UnmarshalJSON, so the wire contract and cache schema can't drift apart.
var releaseTypeLabels = map[ReleaseType]string{
	ReleaseTypeStable:        "stable",
	ReleaseTypeLatestStable:  "latest-stable",
	ReleaseTypePreview:       "preview",
	ReleaseTypeLatestPreview: "latest-preview",
	ReleaseTypeLTS:           "lts",
}

// String returns the release type's canonical label.
func (t ReleaseType) String() string {
	if label, ok := releaseTypeLabels[t]; ok {
		return label
	}
	return "unknown"
}

// MarshalJSON encodes ReleaseType as its string label instead of a raw integer.
func (t ReleaseType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON decodes the string label back into the typed constant (needed
// for cache round-trip).
func (t *ReleaseType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	for rt, label := range releaseTypeLabels {
		if label == s {
			*t = rt
			return nil
		}
	}
	return fmt.Errorf("unknown release type %q", s)
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

// DisplayName returns the version string with a release-type suffix (e.g. "4.15.1 (Latest)").
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

// ShortVersion returns the "major.minor" form (e.g. "4.15"), or "0.0" if unparsable.
func (v *OKDVersion) ShortVersion() string {
	return fmt.Sprintf("%d.%d", v.Major(), v.Minor())
}
