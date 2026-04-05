package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
)

var destroyForce bool

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy a Kubernetes cluster",
	Long:  `Destroy a Kubernetes cluster and all associated infrastructure.`,
	RunE:  runDestroy,
}

func init() {
	destroyCmd.Flags().BoolVarP(&destroyForce, "force", "y", false, "skip confirmation prompt")
}

func runDestroy(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		return err
	}

	tui.Warn("this will destroy cluster '" + cfg.Cluster.Name + "' and all associated resources")

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

	creds := HandleCredentials(cfg)
	defer creds.Zeroize()
	p := CreateOKDProvisionerWithCreds(cfg, creds)

	tui.Info("destroying cluster...")
	startTime := time.Now()

	destroyOpts := &okd.DestroyOptions{
		RemovePackages: true,
	}
	if err := p.Destroy(ctx, cfg, destroyOpts); err != nil {
		return err
	}

	duration := time.Since(startTime).Round(time.Second)
	tui.Info(fmt.Sprintf("cluster destroyed (%s)", duration))

	return nil
}
