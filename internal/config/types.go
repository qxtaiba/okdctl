package config

// DistributionType identifies the Kubernetes distribution to deploy.
type DistributionType string

// Distributions okdctl can deploy.
const (
	DistributionOKD DistributionType = "okd"
)

// ProviderType identifies the infrastructure provider.
type ProviderType string

// Providers okdctl can deploy onto.
const (
	ProviderProxmox ProviderType = "proxmox"
)

// SupportedDistributions returns the distributions okdctl knows how to
// deploy.
func SupportedDistributions() []DistributionType {
	return []DistributionType{
		DistributionOKD,
	}
}

// SupportedProviders returns the infrastructure providers okdctl targets.
func SupportedProviders() []ProviderType {
	return []ProviderType{
		ProviderProxmox,
	}
}
