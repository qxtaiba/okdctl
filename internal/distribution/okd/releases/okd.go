// Package releases provides dynamic version fetching for Kubernetes distributions.
package releases

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/httputil"
)

type OKDVersionFetcher struct {
	httpClient   *http.Client
	diskCacheTTL time.Duration
}

func NewOKDVersionFetcher() *OKDVersionFetcher {
	return &OKDVersionFetcher{
		httpClient:   httputil.NewAPIClient(),
		diskCacheTTL: DiskCacheTTL,
	}
}

func (f *OKDVersionFetcher) FetchVersions(ctx context.Context) ([]OKDReleaseSeries, error) {
	if cached, _ := f.loadFromDiskCache(); cached != nil {
		return cached, nil
	}

	series, err := f.fetchFromNetwork(ctx)
	if err != nil {
		return nil, err
	}

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
