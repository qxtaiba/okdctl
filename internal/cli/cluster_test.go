package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func findSubcommand(t *testing.T, parent *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("%s has no %q subcommand", parent.Name(), name)
	return nil
}

func TestClusterStopStartRegisteredWithExpectedFlags(t *testing.T) {
	for _, name := range []string{"stop", "start"} {
		t.Run(name, func(t *testing.T) {
			cmd := findSubcommand(t, clusterCmd, name)

			for _, flag := range []string{"yes", "confirm-cluster", flagDryRun, "acknowledge-interrupted-op"} {
				if cmd.Flags().Lookup(flag) == nil {
					t.Errorf("missing --%s flag", flag)
				}
			}
			if got := cmd.Flags().ShorthandLookup("y"); got == nil || got.Name != "yes" {
				t.Error("-y must be the shorthand for --yes")
			}
			if cmd.Flags().ShorthandLookup("c") != nil {
				t.Error("--confirm-cluster must stay long-form only (not in the shorthand allowlist)")
			}
			if cmd.RunE == nil {
				t.Error("missing RunE")
			}
		})
	}
}
