// Package releases provides dynamic version fetching for Kubernetes distributions.
package releases

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	utilhttp "github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

type OKDVersionFetcher struct {
	httpClient   *http.Client
	cacheTime    time.Duration // In-memory cache TTL
	cache        []OKDReleaseSeries
	cacheAt      time.Time
	diskCacheTTL time.Duration // On-disk cache TTL
	mu           sync.RWMutex  // Protects cache and cacheAt for thread-safe access
}

func NewOKDVersionFetcher() *OKDVersionFetcher {
	return &OKDVersionFetcher{
		httpClient:   utilhttp.NewAPIClient(),
		cacheTime:    5 * time.Minute,
		diskCacheTTL: DiskCacheTTL,
	}
}

// FetchVersions uses a multi-level caching strategy:
// 1. In-memory cache (5 min TTL) for fast repeated calls in the same process
// 2. On-disk cache (1 hour TTL) to avoid network requests across CLI invocations
func (f *OKDVersionFetcher) FetchVersions(ctx context.Context) ([]OKDReleaseSeries, error) {
	if cached, ok := f.getFromMemoryCache(); ok {
		return cached, nil
	}

	if diskCached, _ := f.loadFromDiskCache(); diskCached != nil {
		f.updateMemoryCache(diskCached)
		return diskCached, nil
	}

	series, err := f.fetchFromNetwork(ctx)
	if err != nil {
		return nil, err
	}

	f.updateMemoryCache(series)
	f.saveToDiskCache(series)

	return series, nil
}

func (f *OKDVersionFetcher) GetLatestStable(ctx context.Context) (OKDVersion, error) {
	series, err := f.FetchVersions(ctx)
	if err != nil {
		return OKDVersion{}, err
	}

	for _, s := range series {
		if s.Latest.Stable {
			return s.Latest, nil
		}
	}

	return OKDVersion{}, fmt.Errorf("no stable version found")
}

func (f *OKDVersionFetcher) GetLatestForMinor(ctx context.Context, major, minor int) (OKDVersion, error) {
	series, err := f.FetchVersions(ctx)
	if err != nil {
		return OKDVersion{}, err
	}

	for _, s := range series {
		if s.Major == major && s.Minor == minor {
			return s.Latest, nil
		}
	}

	return OKDVersion{}, fmt.Errorf("no version found for %d.%d", major, minor)
}
