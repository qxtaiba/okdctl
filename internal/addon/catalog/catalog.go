// Package catalog imports all built-in addons to trigger their init() registration.
// Import this package from main to register all addons at startup.
//
// Adding a new addon:
//  1. Create a new package under internal/addon/catalog/<name>/.
//  2. Implement the addon.Addon interface on a struct: Info, Install, Verify,
//     and Uninstall.
//  3. From an init() function in that package, call addon.Register(&MyAddon{})
//     so the registry picks it up at program start.
//  4. Add a blank import for the new package to this file so it is pulled in
//     whenever the catalog is imported.
//  5. Optionally implement ConfigurableAddon (settings + validation)
//     or WizardProvider (wizard UI fields) for richer integration.
//
// See the flux and secretstore subpackages for reference implementations.
package catalog

import (
	_ "github.com/qxtaiba/okd-proxmox-cli/internal/addon/catalog/flux"
	_ "github.com/qxtaiba/okd-proxmox-cli/internal/addon/catalog/secretstore"
)
