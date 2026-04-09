package releases

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

const (
	// Prevents repeated network requests when the CLI is invoked multiple times.
	DiskCacheTTL = 1 * time.Hour

	cacheFileName = "okd-versions.json"
)

func (f *OKDVersionFetcher) getCacheFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".okdctl", "cache", cacheFileName), nil
}

// Returns nil, nil if cache doesn't exist, is stale, or has errors.
func (f *OKDVersionFetcher) loadFromDiskCache() ([]OKDReleaseSeries, error) {
	cachePath, err := f.getCacheFilePath()
	if err != nil {
		return nil, nil // Gracefully fail - will fetch from network
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Cache doesn't exist yet
		}
		return nil, nil // Other read errors - gracefully fail
	}

	var cache diskCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, nil // Invalid cache format - will refresh
	}

	if time.Since(cache.CachedAt) >= f.diskCacheTTL {
		return nil, nil // Cache is stale
	}

	return cache.Series, nil
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

	_ = system.AtomicWrite(cachePath, data, 0644)
}
