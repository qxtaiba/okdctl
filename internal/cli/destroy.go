package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/tui"
)

var destroyForce bool

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy a Kubernetes cluster",
	Long: `Destroy a Kubernetes cluster and all associated infrastructure.
This operation is idempotent and safe to re-run if a previous destroy was interrupted.`,
	RunE: runDestroy,
}

func init() {
	destroyCmd.Flags().BoolVarP(&destroyForce, "force", "y", false, "skip confirmation prompt")
}

func runDestroy(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	tui.Warn(fmt.Sprintf("this will destroy cluster '%s' and all associated resources", cfg.Cluster.Name))

	if !destroyForce {
		confirmed, err := promptForConfirmation(ctx, "proceed with destroy? [y/N]: ")
		if err != nil {
			return err
		}
		if !confirmed {
			tui.Info("cancelled")
			return nil
		}
	}

	creds := handleCredentials(cfg)
	defer creds.Zeroize()
	p := createOKDProvisioner(cfg, creds)

	tui.Info("destroying cluster...")
	startTime := time.Now()

	if err := p.Destroy(ctx, cfg, true); err != nil {
		return err
	}

	duration := time.Since(startTime).Round(time.Second)
	tui.Info(fmt.Sprintf("cluster destroyed (%s)", duration))

	return nil
}
