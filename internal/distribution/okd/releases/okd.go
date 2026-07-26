// Package releases provides dynamic version fetching for Kubernetes distributions.
package releases

import (
	"context"
	"net/http"
	"time"

	"github.com/qxtaiba/okdctl/internal/httputil"
)

// OKDVersionFetcher resolves OKD release versions, using a short-lived
// on-disk cache to avoid hammering GitHub on repeat invocations.
type OKDVersionFetcher struct {
	httpClient   *http.Client
	diskCacheTTL time.Duration
}

// NewOKDVersionFetcher returns a fetcher configured with the package default
// HTTP timeout and DiskCacheTTL.
func NewOKDVersionFetcher() *OKDVersionFetcher {
	return &OKDVersionFetcher{
		httpClient:   httputil.New(httputil.TimeoutShort),
		diskCacheTTL: DiskCacheTTL,
	}
}

// FetchVersions returns the full list of OKD release series, preferring the
// on-disk cache while it is fresh and falling back to a network fetch.
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
