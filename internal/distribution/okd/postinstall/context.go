package postinstall

type PostInstallContext struct {
	ClusterHealth    *ClusterHealthResult
	KubeVIPVerified  bool
	KubeVipIP        string
	BootstrapCleaned bool
	DNSDeployed      bool
}
