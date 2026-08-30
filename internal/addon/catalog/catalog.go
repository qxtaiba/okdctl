// Package catalog registers every built-in addon via init(); import it from
// main so all addons exist in the registry at startup.
package catalog

import (
	_ "github.com/qxtaiba/okdctl/internal/addon/catalog/flux"        // registers flux
	_ "github.com/qxtaiba/okdctl/internal/addon/catalog/secretstore" // registers secretstore
)
