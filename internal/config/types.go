package config

// DistributionType identifies the Kubernetes distribution to deploy.
type DistributionType string

// Distribution constants enumerate the distributions okdctl can deploy.
const (
	DistributionOKD DistributionType = "okd"
)

// ProviderType identifies the infrastructure provider.
type ProviderType string

// Provider constants enumerate the infrastructure providers okdctl supports.
const (
	ProviderProxmox ProviderType = "proxmox"
)

// SupportedDistributions returns all distribution values okdctl accepts.
func SupportedDistributions() []DistributionType {
	return []DistributionType{
		DistributionOKD,
	}
}

// SupportedProviders returns all provider values okdctl accepts.
func SupportedProviders() []ProviderType {
	return []ProviderType{
		ProviderProxmox,
	}
}
