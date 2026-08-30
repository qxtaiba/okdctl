package cli

import "github.com/spf13/cobra"

// RootCmd returns the root cobra command so offline tooling can walk the command tree.
// Callers must not invoke Execute on it; production entry is cli.Execute().
func RootCmd() *cobra.Command {
	return rootCmd
}
