// Package httputil provides shared *http.Client factories with sensible
// timeouts and optional TLS-skip behaviour for API calls, metadata fetches,
// and large file downloads.
package httputil

import (
	"net/http"
	"time"
)

const (
	TimeoutShort    = 10 * time.Second // For API calls, connectivity checks
	TimeoutMedium   = 30 * time.Second // For fetching metadata, checksums
	TimeoutDownload = 5 * time.Minute  // For file downloads
)

type ClientOption func(*http.Client)

func WithTimeout(d time.Duration) ClientOption {
	return func(c *http.Client) {
		c.Timeout = d
	}
}

func NewClient(opts ...ClientOption) *http.Client {
	c := &http.Client{
		Timeout: TimeoutMedium,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func NewAPIClient(opts ...ClientOption) *http.Client {
	allOpts := append([]ClientOption{WithTimeout(TimeoutShort)}, opts...)
	return NewClient(allOpts...)
}
