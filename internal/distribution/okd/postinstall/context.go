package postinstall

// Flags are independent and may all be true — not an enum, to keep parallel-progress reporting.
type postInstallContext struct {
	ClusterHealth    *ClusterHealthResult
	KubeVIPVerified  bool
	KubeVipIP        string
	BootstrapCleaned bool
	DNSDeployed      bool
}
