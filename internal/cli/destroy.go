package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

var (
	destroyForce    bool
	destroyKeepISOs bool
	destroyDryRun   bool
)

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy a Kubernetes cluster",
	Long: `Destroy a Kubernetes cluster and all associated infrastructure.
This operation is idempotent and safe to re-run if a previous destroy was interrupted.

Use --dry-run to preview the terraform destroy plan without modifying infra.`,
	RunE: runDestroy,
}

func init() {
	destroyCmd.Flags().BoolVarP(&destroyForce, "force", "y", false, "skip confirmation prompt")
	destroyCmd.Flags().BoolVar(&destroyKeepISOs, "keep-isos", false, "do not remove the FCOS ISO from the Proxmox host")
	destroyCmd.Flags().BoolVar(&destroyDryRun, "dry-run", false, "preview terraform destroy plan without running destroy")
}

func runDestroy(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	if destroyDryRun {
		runDestroyDryRun(cfg)
		return nil
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
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	// Destroy writes .tfstate + logs into <projectRoot>/okd-install before
	// tearing down. On partial/cancelled runs the workdir may survive
	// root-owned; restore invoking-user ownership at exit so the user can
	// inspect or retry.
	workDir := filepath.Join(projectRoot, "okd-install")
	defer func() {
		if chownErr := system.ChownTreeToInvokingUser(workDir); chownErr != nil {
			tui.Warn("workdir chown back to user incomplete", tui.LF("err", chownErr))
		}
	}()

	p := createOKDProvisioner(cfg, creds, projectRoot)

	tui.Info("destroying cluster...")
	startTime := time.Now()

	steps, err := p.Destroy(ctx, cfg, true, destroyKeepISOs)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Println(InterruptSummary(steps, "okdctl destroy"))
			return err
		}
		return err
	}

	duration := time.Since(startTime).Round(time.Second)
	tui.Info(fmt.Sprintf("cluster destroyed (%s)", duration))

	return nil
}

// runDestroyDryRun prints what `okdctl destroy` would tear down without
// re-execing as root or touching infra. Always returns nil so the process
// exits 0 — the intent of --dry-run is a survey, not a validation pass.
func runDestroyDryRun(cfg *config.Config) {
	tui.Info(fmt.Sprintf("dry-run: would destroy cluster '%s'", cfg.Cluster.Name))
	tui.Info("dry-run: terraform destroy plan preview is not yet implemented (tracked in roadmap M3)")
	tui.Info("dry-run: the following would be torn down:")
	fmt.Println("  - terraform-provisioned VMs (bootstrap + control-plane + workers)")
	fmt.Println("  - dnsmasq /etc/dnsmasq.d/okd-* drop-in")
	fmt.Println("  - haproxy /etc/haproxy/haproxy.cfg block")
	fmt.Println("  - firewall rules opened by okdctl")
	if !destroyKeepISOs && cfg.Provider.Proxmox != nil {
		fmt.Println("  - FCOS ISOs uploaded to Proxmox storage")
	}
	fmt.Println("  - haproxy, dnsmasq, httpd packages")
	tui.Info("dry-run: no changes made; re-run without --dry-run to execute")
}
