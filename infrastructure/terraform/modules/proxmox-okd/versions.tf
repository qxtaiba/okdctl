terraform {
  # Tightened from ">= 1.2" so untested 2.x or OpenTofu divergence cannot
  # silently apply against this module. CI pins 1.10.3.
  required_version = ">= 1.10, < 2.0"

  required_providers {
    proxmox = {
      source  = "bpg/proxmox"
      version = "~> 0.105.0"
    }
  }
}
