// Package httputil provides *http.Client factories with standard timeouts.
package httputil

import (
	"crypto/tls"
	"errors"
	"net/http"
	"time"
)

// Standard request timeouts tiered by expected response size;
// download.DefaultTimeout aliases TimeoutDownload, so this package owns
// the download-tier value.
const (
	TimeoutShort    = 10 * time.Second // API calls, connectivity checks
	TimeoutDownload = 5 * time.Minute  // File downloads
)

// New returns an *http.Client configured with the given request timeout.
func New(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: capRedirects}
}

// NewOptionalInsecure skips TLS verification only when insecure is true —
// the operator-opt-in path for go-proxmox API clients; new callers must
// register in TestNewInsecureCallerPolicy's allowlist.
func NewOptionalInsecure(insecure bool, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: capRedirects,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}, //nolint:gosec // caller opts in; callers gated by TestNewInsecureCallerPolicy
		},
	}
}

var (
	errTooManyRedirects    = errors.New("httputil: stopped after 5 redirects")
	errCrossHostAuthHeader = errors.New("httputil: refusing cross-host redirect with Authorization header")
	errInsecureRedirect    = errors.New("httputil: refusing HTTPS downgrade redirect")
)

// capRedirects caps redirects at 5 and blocks cross-host redirects carrying
// an Authorization header, since Go's stdlib only strips headers it manages
// internally (an explicit header would otherwise leak to an
// attacker-controlled host).
func capRedirects(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errTooManyRedirects
	}
	if len(via) == 0 {
		return nil
	}
	if via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return errInsecureRedirect
	}
	if req.URL.Host != via[0].URL.Host && req.Header.Get("Authorization") != "" {
		return errCrossHostAuthHeader
	}
	return nil
}
