package postinstall

type PostInstallContext struct {
	ClusterHealth   *ClusterHealthResult
	KubeVIPVerified bool
	KubeVipIP       string
	RouterLBIP      string
	CustomRouterIP  string
}
