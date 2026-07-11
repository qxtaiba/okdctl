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

Run this after a failed deploy, passing the same --log-file you used
during the deploy so the bundle captures the relevant logs.

Pass --quiet to suppress progress logs to stderr when only the bundle
file is needed (e.g. in scripts or CI).`,
	Example: `  okdctl debug-bundle
  okdctl debug-bundle --output-file my-cluster.tgz
  okdctl debug-bundle --skip-must-gather`,
	RunE: runDebugBundle,
}

func init() {
	debugBundleCmd.Flags().StringVar(&debugBundleOutput, flagOutputFile, "", "write bundle to this path (default: okdctl-debug-<ts>.tgz)")
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
