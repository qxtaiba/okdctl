package cluster

// CSR is the minimal view of a Kubernetes CertificateSigningRequest, kept as
// a struct (not []string) so future fields don't break call-site shapes.
type CSR struct {
	Name string
}
