package postinstall

// Flags are independent and may be true simultaneously; do not collapse into
// a single phase enum (loses parallel-progress reporting in the deploy summary).
type postInstallContext struct {
	ClusterHealth    *ClusterHealthResult
	KubeVIPVerified  bool
	KubeVipIP        string
	BootstrapCleaned bool
	DNSDeployed      bool
}
