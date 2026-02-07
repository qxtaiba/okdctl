// Package cluster provides common utilities for Kubernetes cluster operations
// that can be shared across different distributions.
package cluster

// CSR represents a Certificate Signing Request.
type CSR struct {
	Name       string
	Requester  string
	SignerName string
	Approved   bool
	Denied     bool
	Pending    bool
}
