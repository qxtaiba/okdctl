package cluster

// CSR is the minimal view of a Kubernetes CertificateSigningRequest used by
// the client's approval helpers. Today PendingCSRs only returns pending
// records, so a Pending bool would always be true and add nothing — store
// just the name.
type CSR struct {
	Name string
}
