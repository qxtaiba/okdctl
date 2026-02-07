package postinstall

// PostInstallContext holds shared data between postinstall steps.
// Steps write their results here for other steps to consume.
type PostInstallContext struct {
	ClusterHealth   *ClusterHealthResult
	KubeVIPVerified bool
	KubeVipIP       string
}
