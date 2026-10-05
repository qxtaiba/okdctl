package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

var kubeconfigOutput string

var kubeconfigCmd = &cobra.Command{
	Use:   "kubeconfig",
	Short: "Print or export the cluster kubeconfig",
	Long:  "Print the cluster kubeconfig to stdout or write it to a file.",
	Example: `  okdctl kubeconfig                       # print to stdout
  okdctl kubeconfig --output-file ~/.kube/okd.cfg    # write to file`,
	Args: cobra.NoArgs,
	RunE: runKubeconfig,
}

func init() {
	kubeconfigCmd.Flags().StringVar(&kubeconfigOutput, flagOutputFile, "-", "write kubeconfig to file, overwriting it if present ('-' for stdout)")
	rootCmd.AddCommand(kubeconfigCmd)
}

func runKubeconfig(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	workDir := workspace.WorkDir(projectRoot)
	clusterDir := workspace.ClusterConfigDir(workDir)
	src := workspace.KubeconfigPath(clusterDir)

	if !system.FileExists(src) {
		return &errtypes.ConfigError{
			Msg: fmt.Sprintf("kubeconfig not found at %s; run `okdctl deploy` first", src),
			Err: errtypes.ErrConfigMissing,
		}
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read kubeconfig: %w", err)
	}

	if kubeconfigOutput == "" || kubeconfigOutput == "-" {
		_, err = cmd.OutOrStdout().Write(data)
		return err
	}

	if err := system.EnsureDirForFile(kubeconfigOutput); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := system.AtomicWrite(kubeconfigOutput, data, 0o600); err != nil {
		return fmt.Errorf("write kubeconfig: %w", err)
	}
	logutil.Info("kubeconfig written", logutil.LF("path", kubeconfigOutput))
	return nil
}
