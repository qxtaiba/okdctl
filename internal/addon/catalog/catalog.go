// Package catalog imports all built-in addons to trigger their init()
// registration. Import this package from main so every addon is present
// in the registry at program start. See docs/addons/ for the contract
// new addons implement.
package catalog

import (
	_ "github.com/qxtaiba/okdctl/internal/addon/catalog/flux"        // register flux addon
	_ "github.com/qxtaiba/okdctl/internal/addon/catalog/secretstore" // register secretstore addon
)
