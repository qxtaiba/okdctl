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

// Standard request timeouts tiered by expected response size;
// download.DefaultTimeout aliases TimeoutDownload, so this package owns
// the download-tier value.
const (
	TimeoutShort    = 10 * time.Second // API calls, connectivity checks
	TimeoutMedium   = 30 * time.Second // Metadata, checksum fetches
	TimeoutDownload = 5 * time.Minute  // File downloads
)

// New returns an *http.Client configured with the given request timeout.
func New(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: capRedirects}
}

// NewInsecure returns a TLS-skipping client for the bootstrap-window
// kube-vip healthcheck, where the VIP isn't yet in the apiserver SANs.
// Callers MUST attempt the secure path first and fall back here only on
// x509.HostnameError; new callers must register in
// httputil_newinsecure_policy_test.go's allowedPrefixes.
func NewInsecure(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: capRedirects,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // bootstrap self-signed cert; see doc
		},
	}
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

// NewWithCA returns a client whose TLS transport trusts only pool's certs,
// pinned at TLS 1.2 minimum; a chain that doesn't root in pool fails the request.
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
// Authorization header to a different host.
var ErrCrossHostAuthHeader = errors.New("httputil: refusing cross-host redirect with Authorization header")

// capRedirects caps redirects at 5 and blocks cross-host redirects carrying
// an Authorization header, since Go's stdlib only strips headers it manages
// internally (an explicit header would otherwise leak to an
// attacker-controlled host).
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

// KubeconfigCAPool reads kubeconfigPath and returns a cert pool for the
// cluster's CA; the kubeconfig must have at least one cluster entry with
// certificate-authority-data (base64 PEM).
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
