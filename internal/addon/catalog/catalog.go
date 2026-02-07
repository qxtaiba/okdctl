// Package catalog imports all built-in addons to trigger their init() registration.
// Import this package from main to register all addons at startup.
package catalog

import (
	_ "github.com/qxtaiba/okd-proxmox-cli/internal/addon/catalog/flux"
	_ "github.com/qxtaiba/okd-proxmox-cli/internal/addon/catalog/secretstore"
)
