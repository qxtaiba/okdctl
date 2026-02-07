package config

// Implements credentials.ProxmoxConfigProvider to avoid circular imports.

// GetProxmoxHost returns the Proxmox host from configuration.
func (c *Config) GetProxmoxHost() string {
	if c.Provider.Proxmox == nil {
		return ""
	}
	return c.Provider.Proxmox.Host
}

func (c *Config) GetProxmoxInsecure() bool {
	if c.Provider.Proxmox == nil {
		return false
	}
	return c.Provider.Proxmox.Insecure
}

func (c *Config) GetProxmoxAPIToken() string {
	if c.Provider.Proxmox == nil {
		return ""
	}
	return c.Provider.Proxmox.APIToken
}

func (c *Config) GetProxmoxTokenID() string {
	if c.Provider.Proxmox == nil {
		return ""
	}
	return c.Provider.Proxmox.TokenID
}

func (c *Config) GetProxmoxUsername() string {
	if c.Provider.Proxmox == nil {
		return ""
	}
	return c.Provider.Proxmox.Username
}

func (c *Config) GetProxmoxPassword() string {
	if c.Provider.Proxmox == nil {
		return ""
	}
	return c.Provider.Proxmox.Password
}
