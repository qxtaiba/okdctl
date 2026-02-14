package releases

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

func (f *OKDVersionFetcher) fetchFromNetwork(ctx context.Context) ([]OKDReleaseSeries, error) {
	releases, err := f.fetchAllPages(ctx, "okd-project/okd")
	if err != nil {
		return nil, utils.WrapError("failed to fetch OKD releases", err)
	}

	releases = f.deduplicateReleases(releases)
	series := f.parseReleases(releases)

	if len(series) == 0 {
		return nil, fmt.Errorf("no valid OKD versions found in GitHub releases")
	}

	return series, nil
}

func (f *OKDVersionFetcher) fetchFromGitHub(ctx context.Context, repo string, page, perPage int) ([]githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=%d&page=%d", repo, perPage, page)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "okd-proxmox-cli")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	return releases, nil
}

func (f *OKDVersionFetcher) fetchAllPages(ctx context.Context, repo string) ([]githubRelease, error) {
	var allReleases []githubRelease
	page := 1
	perPage := 100

	for {
		select {
		case <-ctx.Done():
			return allReleases, ctx.Err()
		default:
		}

		releases, err := f.fetchFromGitHub(ctx, repo, page, perPage)
		if err != nil {
			if page == 1 {
				return nil, err // Fail completely if first page fails
			}
			// Return what we have if subsequent pages fail
			break
		}

		if len(releases) == 0 {
			break // No more pages
		}

		allReleases = append(allReleases, releases...)

		if len(releases) < perPage {
			break // Last page
		}

		page++

		// Safety limit to prevent infinite loops
		if page > 10 {
			break
		}
	}

	return allReleases, nil
}

func (f *OKDVersionFetcher) deduplicateReleases(releases []githubRelease) []githubRelease {
	seen := make(map[string]bool)
	result := make([]githubRelease, 0, len(releases))

	for _, rel := range releases {
		if !seen[rel.TagName] {
			seen[rel.TagName] = true
			result = append(result, rel)
		}
	}

	return result
}

func (f *OKDVersionFetcher) parseReleases(releases []githubRelease) []OKDReleaseSeries {
	seriesMap := make(map[string]*OKDReleaseSeries)

	for _, rel := range releases {
		if rel.Draft {
			continue
		}

		version := f.parseVersionTag(rel.TagName)
		if version == nil {
			continue
		}

		// Determine stability by tag pattern, not GitHub's prerelease flag.
		// OKD uses ".ec." (engineering candidate) or ".rc." for prereleases.
		// GitHub's prerelease flag is inconsistent - old releases (4.3, 4.4)
		// were incorrectly marked as prerelease when they're actually stable.
		version.Stable = !isPrerelease(rel.TagName)
		version.ReleaseDate = rel.PublishedAt

		key := fmt.Sprintf("%d.%d", version.Major(), version.Minor())

		if series, ok := seriesMap[key]; ok {
			series.Versions = append(series.Versions, *version)
		} else {
			seriesMap[key] = &OKDReleaseSeries{
				Major:    version.Major(),
				Minor:    version.Minor(),
				Versions: []OKDVersion{*version},
			}
		}
	}

	return sortAndClassifySeries(seriesMap)
}

// sortAndClassifySeries converts the series map into a sorted slice, marks the
// latest stable/preview versions within each series, and assigns release types.
func sortAndClassifySeries(seriesMap map[string]*OKDReleaseSeries) []OKDReleaseSeries {
	var result []OKDReleaseSeries
	for _, series := range seriesMap {
		// Sort versions within series (newest first) using proper numeric comparison
		sort.Slice(series.Versions, func(versionIdxA, versionIdxB int) bool {
			return compareVersions(series.Versions[versionIdxA].Version, series.Versions[versionIdxB].Version) > 0
		})

		foundLatestStable := false
		foundLatestPreview := false
		for versionIdx := range series.Versions {
			if series.Versions[versionIdx].Stable && !foundLatestStable {
				series.Versions[versionIdx].Latest = true
				series.Latest = series.Versions[versionIdx]
				foundLatestStable = true
			} else if !series.Versions[versionIdx].Stable && !foundLatestPreview {
				foundLatestPreview = true
			}
		}

		if series.Latest.Version == "" && len(series.Versions) > 0 {
			series.Versions[0].Latest = true
			series.Latest = series.Versions[0]
		}

		result = append(result, *series)
	}

	sort.Slice(result, func(seriesIdxA, seriesIdxB int) bool {
		if result[seriesIdxA].Major != result[seriesIdxB].Major {
			return result[seriesIdxA].Major > result[seriesIdxB].Major
		}
		return result[seriesIdxA].Minor > result[seriesIdxB].Minor
	})

	latestMinor := 0
	if len(result) > 0 {
		latestMinor = result[0].Minor
	}

	for seriesIdx := range result {
		isFirstSeries := seriesIdx == 0
		assignReleaseTypesToSeries(&result[seriesIdx], latestMinor, isFirstSeries)
		syncSeriesLatestVersion(&result[seriesIdx])
	}

	return result
}

func assignReleaseTypesToSeries(series *OKDReleaseSeries, latestMinor int, isFirstSeries bool) {
	for versionIdx := range series.Versions {
		version := &series.Versions[versionIdx]
		isFirstVersion := versionIdx == 0
		assignReleaseTypeToVersion(version, series, latestMinor, isFirstSeries, isFirstVersion)
	}
}

func assignReleaseTypeToVersion(version *OKDVersion, series *OKDReleaseSeries, latestMinor int, isFirstSeries, isFirstVersion bool) {
	if version.Stable {
		version.Type = determineStableReleaseType(version, series.Minor, latestMinor, isFirstSeries)
	} else {
		version.Type = determinePreviewReleaseType(version, series, isFirstVersion)
	}
}

func determineStableReleaseType(version *OKDVersion, seriesMinor, latestMinor int, isFirstSeries bool) ReleaseType {
	if version.Latest && isFirstSeries {
		return ReleaseTypeLatestStable
	}
	if seriesMinor <= latestMinor-2 {
		return ReleaseTypeLTS
	}
	return ReleaseTypeStable
}

func determinePreviewReleaseType(version *OKDVersion, series *OKDReleaseSeries, isFirstVersion bool) ReleaseType {
	// A version is latest preview if explicitly marked or if it's the first version
	// and the series has no stable versions at the top
	if version.Latest || (isFirstVersion && !series.Versions[0].Stable) {
		return ReleaseTypeLatestPreview
	}
	return ReleaseTypePreview
}

func syncSeriesLatestVersion(series *OKDReleaseSeries) {
	if series.Latest.Version == "" {
		return
	}
	for versionIdx := range series.Versions {
		if series.Versions[versionIdx].Version == series.Latest.Version {
			series.Latest = series.Versions[versionIdx]
			return
		}
	}
}

// OKD uses different prerelease patterns across versions:
//   - Modern (4.12+): ".ec." (engineering candidate) or ".rc." (release candidate)
//   - Legacy (4.4 and earlier): "-beta" suffix
func isPrerelease(tag string) bool {
	tagLower := strings.ToLower(tag)
	return strings.Contains(tagLower, ".ec.") ||
		strings.Contains(tagLower, ".rc.") ||
		strings.Contains(tagLower, "-beta")
}

func (f *OKDVersionFetcher) parseVersionTag(tag string) *OKDVersion {
	tag = strings.Trim(tag, "\"'")

	if !strings.Contains(tag, "okd") {
		return nil
	}

	var major, minor int
	if _, err := fmt.Sscanf(tag, "%d.%d", &major, &minor); err != nil || major == 0 {
		return nil
	}

	return &OKDVersion{
		Version: tag,
		Tag:     tag,
	}
}

func compareVersions(a, b string) int {
	partsA := extractVersionParts(a)
	partsB := extractVersionParts(b)

	for i := 0; i < len(partsA) && i < len(partsB); i++ {
		if partsA[i] != partsB[i] {
			return partsA[i] - partsB[i]
		}
	}

	return len(partsA) - len(partsB)
}

func extractVersionParts(version string) []int {
	var parts []int
	var current int
	var inNumber bool
	var digitCount int

	for _, char := range version {
		if char >= '0' && char <= '9' {
			digitCount++
			// Prevent integer overflow by limiting to 10 digits (covers int32 range)
			if digitCount > 10 {
				continue
			}
			current = current*10 + int(char-'0')
			inNumber = true
		} else {
			if inNumber {
				parts = append(parts, current)
				current = 0
				digitCount = 0
				inNumber = false
			}
		}
	}
	if inNumber {
		parts = append(parts, current)
	}

	return parts
}
