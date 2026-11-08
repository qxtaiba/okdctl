package steps

// Networking defaults.
const (
	// DefaultStartIP avoids .100 (default proxmox host) to prevent an ARP
	// conflict with the bootstrap VM.
	DefaultStartIP   = "192.168.1.140"
	DefaultBastionIP = "192.168.1.20"
)

// Boolean select values shared by the data-driven steps.
const (
	valYes = "yes"
	valNo  = "no"
)
