package cli

import "github.com/spf13/cobra"

// RootCmd returns the package-private root cobra command so offline tooling
// (e.g. the CLI reference generator under cmd/okdctl-gen-docs) can walk the
// command tree. Callers must not invoke Execute on the returned command;
// production entry is cli.Execute().
func RootCmd() *cobra.Command {
	return rootCmd
}
