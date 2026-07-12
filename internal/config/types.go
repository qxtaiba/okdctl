package config

// DistributionType identifies the Kubernetes distribution to deploy.
// Scaffolding: single-variant by design — multi-distribution support is
// deliberately out of scope; the typed enum preserves the API surface so
// call sites need no change if a second variant ever lands.
type DistributionType string

// Distributions okdctl can deploy.
const (
	DistributionOKD DistributionType = "okd"
)

// ProviderType identifies the infrastructure provider.
// Scaffolding: single-variant by design — multi-provider support is
// deliberately out of scope; ProviderConfig stays Proxmox-shaped. The
// typed enum is kept for call-site stability, not future expansion.
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
