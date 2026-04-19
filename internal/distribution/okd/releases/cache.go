package releases

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/system"
)

const (
	// DiskCacheTTL bounds how long the on-disk OKD release cache is reused
	// before a fresh network fetch is required.
	DiskCacheTTL = 1 * time.Hour

	cacheFileName = "okd-versions.json"
)

// Cache lives under the invoking user's home so `okdctl releases list` run
// as a non-root user later can still read it, even when the cache was
// populated during a root-mode deploy.
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
		return nil // Gracefully fail - will fetch from network
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil // Cache doesn't exist or read error - gracefully fail
	}

	var cache diskCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil // Invalid cache format - will refresh
	}

	if time.Since(cache.CachedAt) >= f.diskCacheTTL {
		return nil // Cache is stale
	}

	return cache.Series
}

func (f *OKDVersionFetcher) saveToDiskCache(series []OKDReleaseSeries) {
	cachePath, err := f.getCacheFilePath()
	if err != nil {
		return // Can't determine cache path, skip silently
	}

	cache := diskCache{
		CachedAt: time.Now(),
		Series:   series,
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return // Serialization failed, skip silently
	}

	_ = system.WriteAsInvokingUser(cachePath, data, 0o644)
}
