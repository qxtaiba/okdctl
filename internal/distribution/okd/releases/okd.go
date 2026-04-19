// Package releases provides dynamic version fetching for Kubernetes distributions.
package releases

import (
	"context"
	"fmt"
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

// GetLatestStable returns the newest OKDVersion flagged as stable across
// all release series.
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

// GetLatestForMinor returns the latest version in the given major.minor
// series, or an error if no series matches.
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
