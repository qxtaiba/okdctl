package postinstall

type PostInstallContext struct {
	ClusterHealth    *ClusterHealthResult
	KubeVIPVerified  bool
	KubeVipIP        string
	APIDNSSwitched   bool
	BootstrapCleaned bool
	RouterLBIP       string
	CustomRouterIP   string
}
