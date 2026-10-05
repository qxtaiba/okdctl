package cli

import (
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/lifecycle"
)

// demoExecStepDelay paces each lifecycle demo execution event so the exec
// screen has visible in-progress rows to screenshot instead of finishing instantly.
const demoExecStepDelay = 350 * time.Millisecond

// demoConfig returns the static config OKDCTL_WIZARD_DEMO drives the
// lifecycle wizard against, sharing lifecycle.DemoClusterName with
// DemoHooks' fixture so every node the operator picks resolves.
func demoConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = lifecycle.DemoClusterName
	return cfg
}
