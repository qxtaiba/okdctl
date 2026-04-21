package releases

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"golang.org/x/mod/semver"
)

func (f *OKDVersionFetcher) fetchFromNetwork(ctx context.Context) ([]OKDReleaseSeries, error) {
	releases, err := f.fetchAllPages(ctx, "okd-project/okd")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OKD releases: %w", err)
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

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "okdctl")

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
		// Sort versions within series (newest first) using proper numeric
		// comparison. When two versions compare equal by their numeric parts
		// (e.g. tag variants on the same release), fall back to the GitHub
		// published_at timestamp (newest first), and finally to the raw tag
		// string, so the ordering is stable regardless of the input order
		// returned by the GitHub API.
		slices.SortFunc(series.Versions, func(a, b OKDVersion) int {
			return cmp.Or(
				-semver.Compare("v"+a.Version, "v"+b.Version), // descending by semver
				b.ReleaseDate.Compare(a.ReleaseDate),          // newest first
				strings.Compare(b.Version, a.Version),         // descending raw tag
			)
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

	slices.SortFunc(result, func(a, b OKDReleaseSeries) int {
		if a.Major != b.Major {
			return b.Major - a.Major // descending
		}
		return b.Minor - a.Minor // descending
	})

	latestMinor := 0
	if len(result) > 0 {
		latestMinor = result[0].Minor
	}

	for seriesIdx := range result {
		isFirstSeries := seriesIdx == 0
		for versionIdx := range result[seriesIdx].Versions {
			v := &result[seriesIdx].Versions[versionIdx]
			switch {
			case v.Stable && v.Latest && isFirstSeries:
				v.Type = ReleaseTypeLatestStable
			case v.Stable && result[seriesIdx].Minor <= latestMinor-2:
				v.Type = ReleaseTypeLTS
			case v.Stable:
				v.Type = ReleaseTypeStable
			case v.Latest || (versionIdx == 0 && !result[seriesIdx].Versions[0].Stable):
				v.Type = ReleaseTypeLatestPreview
			default:
				v.Type = ReleaseTypePreview
			}
		}
		// Sync series.Latest with the (possibly updated) version entry.
		for versionIdx := range result[seriesIdx].Versions {
			if result[seriesIdx].Versions[versionIdx].Version == result[seriesIdx].Latest.Version {
				result[seriesIdx].Latest = result[seriesIdx].Versions[versionIdx]
				break
			}
		}
	}

	return result
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
