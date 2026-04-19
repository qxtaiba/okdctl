package cluster

// CSR is the minimal view of a Kubernetes CertificateSigningRequest used by
// the client's approval helpers.
type CSR struct {
	Name    string
	Pending bool
}
