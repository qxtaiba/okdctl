package cluster

import certificatesv1 "k8s.io/api/certificates/v1"

// CSR retains the observed request used for policy checks and version-guarded approval.
type CSR struct {
	Name    string
	Request certificatesv1.CertificateSigningRequest
}
