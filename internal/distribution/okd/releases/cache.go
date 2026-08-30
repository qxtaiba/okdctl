package releases

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/system"
)

const (
	// defaultDiskCacheTTL: how long the on-disk cache is reused before a refetch.
	defaultDiskCacheTTL = 1 * time.Hour

	cacheFileName = "okd-versions.json"
)

// Cache lives under the invoking user's home so a later non-root `releases
// list` can read it even if a root-mode deploy populated it.
func (f *OKDVersionFetcher) getCacheFilePath() (string, error) {
	homeDir, err := system.InvokingUserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".okdctl", "cache", cacheFileName), nil
}

// Returns nil if cache doesn't exist, is stale, or has errors.
func (f *OKDVersionFetcher) loadFromDiskCache() []OKDReleaseSeries {
	cachePath, err := f.getCacheFilePath()
	if err != nil {
		return nil
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil
	}

	var cache diskCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}

	if cache.Schema != diskCacheSchema {
		return nil
	}

	if time.Since(cache.CachedAt) >= f.diskCacheTTL {
		return nil
	}

	return cache.Series
}

func (f *OKDVersionFetcher) saveToDiskCache(series []OKDReleaseSeries) {
	cachePath, err := f.getCacheFilePath()
	if err != nil {
		return
	}

	cache := diskCache{
		Schema:   diskCacheSchema,
		CachedAt: time.Now(),
		Series:   series,
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return
	}

	// 0o600: cache data is public, but tightening avoids surprising
	// readability if a future field lands here.
	_ = system.WriteAsInvokingUser(cachePath, data, 0o600)
}
