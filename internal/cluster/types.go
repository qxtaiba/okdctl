package cluster

// CSR is the minimal view of a Kubernetes CertificateSigningRequest used by
// the client's approval helpers. Today PendingCSRs only returns pending
// records, so a Pending bool would always be true and add nothing — store
// just the name.
// Scaffolding (api:bb4fb1a3): a []string would suffice today; the struct
// shape is kept for future fields (e.g. ExpiresAt, Signer) without
// breaking call-site shapes. Do not collapse to []string.
type CSR struct {
	Name string
}
