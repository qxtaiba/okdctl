// Package httputil provides *http.Client factories with standard timeouts.
package httputil

import (
	"net/http"
	"time"
)

const (
	TimeoutShort    = 10 * time.Second // API calls, connectivity checks
	TimeoutMedium   = 30 * time.Second // Metadata, checksum fetches
	TimeoutDownload = 5 * time.Minute  // File downloads
)

func New(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}
