package system

import (
	"crypto/tls"
	"net/http"
	"time"
)

// Default timeout values for different use cases.
const (
	TimeoutShort    = 10 * time.Second // For API calls, connectivity checks
	TimeoutMedium   = 30 * time.Second // For fetching metadata, checksums
	TimeoutDownload = 5 * time.Minute  // For file downloads
)

// ClientOption configures an HTTP client.
type ClientOption func(*http.Client)

// WithTimeout sets the client timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *http.Client) {
		c.Timeout = d
	}
}

// WithInsecureSkipVerify disables TLS certificate verification.
// Use with caution - only for self-signed certificates in trusted environments.
// If the transport is not *http.Transport, this option is silently ignored.
func WithInsecureSkipVerify() ClientOption {
	return func(c *http.Client) {
		if c.Transport == nil {
			c.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			return
		}
		t, ok := c.Transport.(*http.Transport)
		if !ok {
			// Cannot modify non-standard transport, skip silently
			return
		}
		if t.TLSClientConfig == nil {
			t.TLSClientConfig = &tls.Config{}
		}
		t.TLSClientConfig.InsecureSkipVerify = true
	}
}

// NewClient creates a new HTTP client with the given options.
// Default timeout is TimeoutMedium (30 seconds).
func NewClient(opts ...ClientOption) *http.Client {
	c := &http.Client{
		Timeout: TimeoutMedium,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// NewAPIClient creates an HTTP client configured for API calls.
// Uses TimeoutShort (10 seconds) by default.
func NewAPIClient(opts ...ClientOption) *http.Client {
	allOpts := append([]ClientOption{WithTimeout(TimeoutShort)}, opts...)
	return NewClient(allOpts...)
}
