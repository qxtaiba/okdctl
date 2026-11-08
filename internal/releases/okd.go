// Package releases fetches, classifies, and caches the OKD release catalog
// from GitHub.
package releases

import (
	"context"
	"net/http"
	"time"

	"github.com/qxtaiba/okdctl/internal/httputil"
)

// OKDVersionFetcher resolves OKD release versions via a short-lived on-disk cache.
type OKDVersionFetcher struct {
	httpClient   *http.Client
	diskCacheTTL time.Duration
}

// NewOKDVersionFetcher returns a fetcher with the default HTTP timeout and disk-cache TTL.
func NewOKDVersionFetcher() *OKDVersionFetcher {
	return &OKDVersionFetcher{
		httpClient:   httputil.New(httputil.TimeoutShort),
		diskCacheTTL: defaultDiskCacheTTL,
	}
}

// FetchVersions returns OKD release series, preferring a fresh on-disk cache over a network fetch.
func (f *OKDVersionFetcher) FetchVersions(ctx context.Context) ([]OKDReleaseSeries, error) {
	if cached := f.loadFromDiskCache(); cached != nil {
		return cached, nil
	}

	series, err := f.fetchFromNetwork(ctx)
	if err != nil {
		return nil, err
	}

	f.saveToDiskCache(series)
	return series, nil
}
