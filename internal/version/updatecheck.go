package version

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/mod/semver"

	"github.com/qxtaiba/okdctl/internal/fetchplan"
	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/system"
)

const (
	cacheTTL    = 24 * time.Hour
	httpTimeout = 4 * time.Second
)

// CheckResult carries the outcome of a background update check.
// LatestTag is empty when no newer version was found or the check failed.
type CheckResult struct {
	LatestTag string
}

type cacheEntry struct {
	LastChecked time.Time `json:"last_checked"`
	LatestTag   string    `json:"latest_tag"`
}

// BackgroundCheck starts a goroutine that checks for a newer release.
// It returns a buffered channel (capacity 1) that receives exactly one
// CheckResult. If OKDCTL_NO_UPDATE_CHECK=1 the goroutine is not started
// and the channel already holds a zero result. The resolver routes the
// GitHub API call through any mirror or env override.
func BackgroundCheck(ctx context.Context, resolver fetchplan.Resolver) <-chan CheckResult {
	ch := make(chan CheckResult, 1)

	if os.Getenv("OKDCTL_NO_UPDATE_CHECK") == "1" {
		ch <- CheckResult{}
		return ch
	}

	go func() {
		ch <- runCheck(ctx, resolver)
	}()

	return ch
}

func runCheck(ctx context.Context, resolver fetchplan.Resolver) CheckResult {
	current := Version
	if !semver.IsValid(canonicalTag(current)) {
		return CheckResult{}
	}

	if entry, ok := loadCache(); ok {
		if isNewer(current, entry.LatestTag) {
			return CheckResult{LatestTag: entry.LatestTag}
		}
		return CheckResult{}
	}

	tag, err := fetchLatest(ctx, resolver)
	if err != nil {
		slog.Debug("update check fetch failed", "err", err)
		return CheckResult{}
	}

	if err := saveCache(tag); err != nil {
		slog.Debug("update check cache save failed", "err", err)
	}

	if isNewer(current, tag) {
		return CheckResult{LatestTag: tag}
	}
	return CheckResult{}
}

func fetchLatest(ctx context.Context, resolver fetchplan.Resolver) (string, error) {
	if resolver == nil {
		resolver = fetchplan.DefaultResolver{}
	}
	url, err := resolver.ResolveBlob(fetchplan.BuildUpdateCheckPlan().HTTPS[0])
	if err != nil {
		return "", fmt.Errorf("resolve update check URL: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := httputil.New(httpTimeout).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return payload.TagName, nil
}

func loadCache() (cacheEntry, bool) {
	path, err := cachePath()
	if err != nil {
		return cacheEntry{}, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cacheEntry{}, false
	}

	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return cacheEntry{}, false
	}

	if time.Since(entry.LastChecked) > cacheTTL {
		return cacheEntry{}, false
	}
	return entry, true
}

func saveCache(tag string) error {
	path, err := cachePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	entry := cacheEntry{LastChecked: time.Now().UTC(), LatestTag: tag}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	return system.AtomicWrite(path, data, 0o600)
}

func cachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "okdctl", "update-check.json"), nil
}

func isNewer(current, latestTag string) bool {
	c := canonicalTag(current)
	l := canonicalTag(latestTag)
	if !semver.IsValid(c) || !semver.IsValid(l) {
		return false
	}
	return semver.Compare(l, c) > 0
}

func canonicalTag(v string) string {
	if v == "" {
		return v
	}
	if v[0] != 'v' {
		return "v" + v
	}
	return v
}
