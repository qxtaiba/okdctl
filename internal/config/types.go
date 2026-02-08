package config

type DistributionType string

const (
	DistributionOKD DistributionType = "okd"
)

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
