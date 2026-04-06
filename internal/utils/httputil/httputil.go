// Package httputil provides shared *http.Client factories with sensible
// timeouts and optional TLS-skip behaviour for API calls, metadata fetches,
// and large file downloads.
package httputil

import (
	"crypto/tls"
	"log"
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

func WithInsecureSkipVerify() ClientOption {
	return func(c *http.Client) {
		log.Printf("WARNING: TLS certificate verification disabled for HTTP client")
		if c.Transport == nil {
			c.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			return
		}
		t, ok := c.Transport.(*http.Transport)
		if !ok {
			return
		}
		if t.TLSClientConfig == nil {
			t.TLSClientConfig = &tls.Config{}
		}
		t.TLSClientConfig.InsecureSkipVerify = true
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
