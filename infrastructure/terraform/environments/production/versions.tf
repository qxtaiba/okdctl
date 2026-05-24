terraform {
  required_version = ">= 1.10, < 2.0"

  required_providers {
    proxmox = {
      source  = "bpg/proxmox"
      version = "~> 0.107.0"
    }
  }
}

# insecure = false is explicit so PROXMOX_VE_INSECURE cannot silently
# disable TLS verification in production. Endpoint and credentials are
# still sourced from PROXMOX_VE_* env vars.
provider "proxmox" {
  insecure = false
}
