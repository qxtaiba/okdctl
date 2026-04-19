package postinstall

// PostInstallContext carries state between post-install steps (health
// results, VIP status, whether DNS has been updated, etc.).
//
//nolint:revive // stutter-named type is the established internal API; rename deferred to a dedicated refactor
type PostInstallContext struct {
	ClusterHealth    *ClusterHealthResult
	KubeVIPVerified  bool
	KubeVipIP        string
	BootstrapCleaned bool
	DNSDeployed      bool
}
