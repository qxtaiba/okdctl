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
const (
	TimeoutShort    = 10 * time.Second // API calls, connectivity checks
	TimeoutMedium   = 30 * time.Second // Metadata, checksum fetches
	TimeoutDownload = 5 * time.Minute  // File downloads
)

// New returns an *http.Client configured with the given request timeout.
func New(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: capRedirects}
}

// NewInsecure returns a client that skips TLS verification. Use only for
// probes against endpoints whose cert is not yet trusted (bootstrap-phase
// kube-vip, cluster healthz served by a self-signed kube-apiserver before
// the VIP appears in its SANs).
func NewInsecure(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: capRedirects,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // bootstrap self-signed cert; see doc
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
		return errors.New("httputil: stopped after 5 redirects")
	}
	if req.URL.Host != via[0].URL.Host && req.Header.Get("Authorization") != "" {
		return errors.New("httputil: refusing cross-host redirect with Authorization header")
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
