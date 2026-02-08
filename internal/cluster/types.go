package cluster

type CSR struct {
	Name       string
	Requester  string
	SignerName string
	Approved   bool
	Denied     bool
	Pending    bool
}
