terraform {
  required_version = ">= 1.2"

  required_providers {
    proxmox = {
      source  = "bpg/proxmox"
      version = "~> 0.103.0"
    }
  }
}
