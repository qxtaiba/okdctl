package cli

import (
	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/debugbundle"
)

var (
	debugBundleOutput         string
	debugBundleSkipMustGather bool
)

var debugBundleCmd = &cobra.Command{
	Use:   "debug-bundle",
	Short: "Collect a support bundle for troubleshooting",
	Long: `Collect a tarball containing redacted configuration, recent log
output, oc adm must-gather results, terraform state summary,
okdctl doctor results, and system metadata.

The output is safe to attach to a support ticket — credentials are
redacted and the raw terraform state file is never included.

Run this after a failed deploy. deploy, destroy, and cleanup append
their full log to okdctl.log in the working directory by default and
the bundle picks it up automatically; pass the same --log-file the
failing run used only if it overrode that default.

Pass --quiet to suppress progress logs to stderr when only the bundle
file is needed (e.g. in scripts or CI).`,
	Example: `  okdctl debug-bundle
  okdctl debug-bundle --output-file my-cluster.tgz
  okdctl debug-bundle --skip-must-gather`,
	Args: cobra.NoArgs,
	RunE: runDebugBundle,
}

func init() {
	debugBundleCmd.Flags().StringVar(&debugBundleOutput, flagOutputFile, "", "write bundle to this path; empty auto-generates okdctl-debug-<ts>.tgz in the cwd; overwrites an existing file at that path")
	debugBundleCmd.Flags().BoolVar(&debugBundleSkipMustGather, "skip-must-gather", false, "skip oc adm must-gather (faster, omits cluster diagnostics)")
	rootCmd.AddCommand(debugBundleCmd)
}

func runDebugBundle(cmd *cobra.Command, _ []string) error {
	return debugbundle.Write(cmd.Context(), debugbundle.Options{
		OutPath:        debugBundleOutput,
		LoadConfig:     func() (*config.Config, error) { return loadConfig(cfgFile) },
		ProjectRoot:    resolveProjectRoot,
		LogFile:        logFile,
		SkipMustGather: debugBundleSkipMustGather,
	})
}
