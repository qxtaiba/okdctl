package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
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

// runNodeManageDemo drives the lifecycle wizard against lifecycle.DemoHooks'
// static six-node fixture, so OKDCTL_WIZARD_DEMO can screenshot every screen
// without a live Proxmox/OKD cluster.
func runNodeManageDemo(cmd *cobra.Command) error {
	cfg := demoConfig()
	st := &lifecycle.State{Cfg: cfg}
	chrome := wizard.FlowChrome{
		Tagline: "okd over proxmox, the easy way",
		Badge:   func(c *config.Config) string { return c.Cluster.Name },
	}
	result, err := wizard.RunFlow(cmd.Context(), lifecycle.NewSteps(st, lifecycle.DemoHooks(demoExecStepDelay)), cfg, chrome)
	if err != nil {
		return &errtypes.ConfigError{Msg: "lifecycle wizard", Err: err}
	}
	return reportLifecycleOutcome(cmd, result, st)
}
