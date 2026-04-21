package config

// DistributionType identifies the Kubernetes distribution to deploy.
type DistributionType string

const (
	DistributionOKD DistributionType = "okd"
)

// ProviderType identifies the infrastructure provider.
type ProviderType string

const (
	ProviderProxmox ProviderType = "proxmox"
)

func SupportedDistributions() []DistributionType {
	return []DistributionType{
		DistributionOKD,
	}
}

func SupportedProviders() []ProviderType {
	return []ProviderType{
		ProviderProxmox,
	}
}
