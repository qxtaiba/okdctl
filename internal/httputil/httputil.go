// Package httputil provides *http.Client factories with standard timeouts.
package httputil

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
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

// NewWithCA returns a client that validates server certificates against the
// given x509 CA pool. Use post-install probes once the cluster CA is
// available (e.g. parsed out of the kubeconfig at clusterDir/auth/kubeconfig).
// A nil pool falls back to the system default.
func NewWithCA(pool *x509.CertPool, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    pool,
			},
		},
	}
}

// CAPoolFromKubeconfig reads the kubeconfig at path and returns an
// x509.CertPool populated from the cluster's certificate-authority-data.
// Returns an error if the file cannot be read, the kubeconfig has no
// clusters, or the CA bundle fails to parse.
func CAPoolFromKubeconfig(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read kubeconfig %s: %w", path, err)
	}
	var kc struct {
		Clusters []struct {
			Cluster struct {
				CertificateAuthorityData string `json:"certificate-authority-data"`
			} `json:"cluster"`
		} `json:"clusters"`
	}
	if err := yaml.Unmarshal(data, &kc); err != nil {
		return nil, fmt.Errorf("parse kubeconfig %s: %w", path, err)
	}
	if len(kc.Clusters) == 0 {
		return nil, fmt.Errorf("kubeconfig %s has no clusters", path)
	}
	raw, err := base64.StdEncoding.DecodeString(kc.Clusters[0].Cluster.CertificateAuthorityData)
	if err != nil {
		return nil, fmt.Errorf("decode CA bundle from %s: %w", path, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(raw) {
		return nil, fmt.Errorf("no valid PEM certs in CA bundle from %s", path)
	}
	return pool, nil
}
