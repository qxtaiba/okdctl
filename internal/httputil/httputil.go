// Package httputil provides *http.Client factories with standard timeouts.
package httputil

import (
	"crypto/tls"
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

// NewInsecure returns a client that skips TLS verification. Use only for
// probes against endpoints whose cert is not yet trusted (bootstrap-phase
// kube-vip, cluster healthz served by a self-signed kube-apiserver).
func NewInsecure(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // bootstrap self-signed cert; see doc
		},
	}
}
