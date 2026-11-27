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
	// TimeoutResponseHeader bounds dial, TLS handshake and response headers
	// on NewOptionalInsecure's transport, matching internal/download; the
	// body answers to ctx alone so uploads and long polls stay unbounded.
	TimeoutResponseHeader = 30 * time.Second
)

// New returns an *http.Client configured with the given request timeout.
func New(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: capRedirects}
}

// NewOptionalInsecure clones http.DefaultTransport (proxy, dial timeout)
// and skips TLS verification only when insecure is true, the operator
// opt-in for Proxmox API clients. A zero timeout leaves the whole request
// unbounded, which is safe because the transport times out a silent peer.
func NewOptionalInsecure(insecure bool, timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: insecure} //nolint:gosec // caller opts in
	transport.TLSHandshakeTimeout = TimeoutResponseHeader
	transport.ResponseHeaderTimeout = TimeoutResponseHeader
	return &http.Client{Timeout: timeout, CheckRedirect: capRedirects, Transport: transport}
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
