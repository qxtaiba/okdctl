terraform {
  # Tightened from >= 1.2 to block untested 2.x/OpenTofu; CI pins 1.10.3.
  required_version = ">= 1.10, < 2.0"

  required_providers {
    proxmox = {
      source  = "bpg/proxmox"
      version = "~> 0.111.0"
    }
  }
}
