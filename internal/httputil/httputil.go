// Package httputil provides *http.Client factories with standard timeouts.
package httputil

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"sigs.k8s.io/yaml"
)

// Standard request timeouts tiered by expected response size.
// TimeoutShort/TimeoutMedium/TimeoutDownload form a symmetric latency tier;
// download.DefaultTimeout aliases TimeoutDownload, so this package owns the
// download-tier value.
const (
	TimeoutShort    = 10 * time.Second // API calls, connectivity checks
	TimeoutMedium   = 30 * time.Second // Metadata, checksum fetches
	TimeoutDownload = 5 * time.Minute  // File downloads
)

// New returns an *http.Client configured with the given request timeout.
func New(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: capRedirects}
}

// NewInsecure returns a client that skips TLS verification. It exists
// exclusively for the bootstrap-window kube-vip healthcheck, where the
// VIP is not yet in the apiserver SANs and x509.HostnameError is the
// expected failure mode on the secure path.
//
// Contract: callers MUST attempt the secure path first (httputil.New or
// httputil.NewWithCA) and reach NewInsecure only on x509.HostnameError.
// Falling back on any other error class (network timeout, 5xx, connection
// refused) is incorrect and must not use this client.
//
// Adding a new caller requires two things:
//  1. Add the caller's package path to allowedPrefixes in
//     httputil_newinsecure_policy_test.go (TestNewInsecureCallerPolicy).
//  2. Add a parallel test that asserts (a) the secure path succeeds when
//     the cert is valid and (b) the insecure fallback is reached only on
//     x509.HostnameError. See internal/distribution/okd/postinstall/
//     haproxy_test.go:97-158 (TestRemoveHAProxy_KubeVIPHealthcheck) for
//     the template.
func NewInsecure(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: capRedirects,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // bootstrap self-signed cert; see doc
		},
	}
}

// NewOptionalInsecure returns a client that skips TLS verification only when
// insecure is true — the operator-opt-in path for the go-proxmox API clients
// (host probe, power-cycler, wizard discovery). The standard redirect policy
// is always installed and is load-bearing here: go-proxmox attaches auth per
// request, so an uncapped redirect chain could walk the credential cross-host.
//
// Adding a new caller requires adding its package path to the
// NewOptionalInsecure allowlist in TestNewInsecureCallerPolicy
// (httputil_newinsecure_policy_test.go).
func NewOptionalInsecure(insecure bool, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: capRedirects,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}, //nolint:gosec // caller opts in; callers gated by TestNewInsecureCallerPolicy
		},
	}
}

// NewWithCA returns a client whose TLS transport trusts only the certificates
// in pool. MinVersion is TLS 1.2; the server must present a cert whose chain
// roots in pool or the request fails.
func NewWithCA(pool *x509.CertPool, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: capRedirects,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    pool,
				MinVersion: tls.VersionTLS12,
			},
		},
	}
}

// ErrTooManyRedirects is returned by capRedirects after 5 consecutive hops.
var ErrTooManyRedirects = errors.New("httputil: stopped after 5 redirects")

// ErrCrossHostAuthHeader is returned when a redirect would carry an
// Authorization header to a different host. Go's stdlib strips headers it
// manages internally on cross-host redirects, but a header set via
// req.Header.Set survives — without this guard it would silently forward
// a bearer token to an attacker-controlled destination.
var ErrCrossHostAuthHeader = errors.New("httputil: refusing cross-host redirect with Authorization header")

// capRedirects is the CheckRedirect policy installed on every client this
// package returns. It caps at 5 redirects and refuses to follow any
// cross-host redirect that carries an Authorization header — Go's stdlib
// strips Authorization on cross-host redirects for headers it manages
// internally, but a header set explicitly via req.Header.Set survives,
// which would silently leak a bearer token to an attacker-controlled host.
// Five redirects is half the Go default of ten and matches hardened client
// guidance; legitimate CDN chains rarely exceed two hops.
func capRedirects(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return ErrTooManyRedirects
	}
	if req.URL.Host != via[0].URL.Host && req.Header.Get("Authorization") != "" {
		return ErrCrossHostAuthHeader
	}
	return nil
}

// kubeconfigCA is the minimal kubeconfig shape needed to extract the cluster CA.
type kubeconfigCA struct {
	Clusters []struct {
		Cluster struct {
			CertificateAuthorityData string `json:"certificate-authority-data"`
		} `json:"cluster"`
	} `json:"clusters"`
}

// KubeconfigCAPool reads kubeconfigPath and returns a cert pool containing the
// cluster's certificate authority. The kubeconfig must have at least one cluster
// entry with certificate-authority-data (base64-encoded PEM).
func KubeconfigCAPool(kubeconfigPath string) (*x509.CertPool, error) {
	raw, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("read kubeconfig: %w", err)
	}

	var kc kubeconfigCA
	if err := yaml.Unmarshal(raw, &kc); err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}

	if len(kc.Clusters) == 0 || kc.Clusters[0].Cluster.CertificateAuthorityData == "" {
		return nil, fmt.Errorf("kubeconfig has no certificate-authority-data")
	}

	pemBytes, err := base64.StdEncoding.DecodeString(kc.Clusters[0].Cluster.CertificateAuthorityData)
	if err != nil {
		return nil, fmt.Errorf("decode certificate-authority-data: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, fmt.Errorf("no valid PEM certificates in certificate-authority-data")
	}

	return pool, nil
}
