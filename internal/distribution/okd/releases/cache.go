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

func (f *OKDVersionFetcher) isCacheFresh() bool {
	return f.cache != nil && time.Since(f.cacheAt) < f.cacheTime
}

func (f *OKDVersionFetcher) updateMemoryCache(series []OKDReleaseSeries) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cache = series
	f.cacheAt = time.Now()
}

func (f *OKDVersionFetcher) getFromMemoryCache() ([]OKDReleaseSeries, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.isCacheFresh() {
		// Return a copy to avoid TOCTOU race with updateMemoryCache
		result := make([]OKDReleaseSeries, len(f.cache))
		copy(result, f.cache)
		return result, true
	}
	return nil, false
}
