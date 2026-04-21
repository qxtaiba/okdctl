package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

var (
	destroyForce          bool
	destroyKeepISOs       bool
	destroyDryRun         bool
	destroyConfirmCluster string
)

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy a Kubernetes cluster",
	Long: `Destroy a Kubernetes cluster and all associated infrastructure.
This operation is idempotent and safe to re-run if a previous destroy was interrupted.

Use --dry-run to preview the terraform destroy plan without modifying infra.`,
	Example: `  okdctl destroy                              # interactive prompt
  okdctl destroy --yes --confirm-cluster=prod # scripted destroy
  okdctl destroy --dry-run`,
	RunE: runDestroy,
}

func init() {
	// Primary flag is --yes (matches the other commands' idiom); --force is
	// kept as an alias so existing scripts do not break.
	destroyCmd.Flags().BoolVarP(&destroyForce, "yes", "y", false, "skip confirmation prompt")
	destroyCmd.Flags().BoolVar(&destroyForce, "force", false, "deprecated alias for --yes")
	if err := destroyCmd.Flags().MarkDeprecated("force", "use --yes instead"); err != nil {
		panic(err) // flag is statically defined above; unreachable
	}
	destroyCmd.Flags().BoolVar(&destroyKeepISOs, "keep-isos", false, "do not remove the FCOS ISO from the Proxmox host")
	destroyCmd.Flags().BoolVar(&destroyDryRun, "dry-run", false, "preview terraform destroy plan without running destroy")
	destroyCmd.Flags().StringVar(&destroyConfirmCluster, "confirm-cluster", "",
		"required with --yes; must equal cfg.Cluster.Name (typo guard for scripted destroys)")
}

func runDestroy(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	tui.SetRunID(uuid.NewString())

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	if destroyDryRun {
		return runDestroyDryRun(ctx, cfg)
	}

	tui.Warn(fmt.Sprintf("this will destroy cluster '%s' and all associated resources", cfg.Cluster.Name))

	// Typo guard for non-interactive destroys: --confirm-cluster is REQUIRED
	// with --yes and must match the live cluster name. Skipping the prompt
	// without asserting the cluster name is the documented foot-gun for
	// scripts that get pointed at the wrong config file.
	if destroyForce {
		if destroyConfirmCluster == "" {
			return &errtypes.ConfigError{
				Msg: fmt.Sprintf("--yes requires --confirm-cluster=%q to guard against scripted destroys against the wrong cluster", cfg.Cluster.Name),
			}
		}
		if destroyConfirmCluster != cfg.Cluster.Name {
			return &errtypes.ConfigError{
				Msg: fmt.Sprintf("--confirm-cluster %q does not match config cluster %q; refusing destroy",
					destroyConfirmCluster, cfg.Cluster.Name),
			}
		}
	}

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

	p := createOKDProvisionerWithOpts(cfg, creds, projectRoot)

	tui.Info("destroying cluster...")
	startTime := time.Now()

	steps, err := p.Destroy(ctx, cfg, true, destroyKeepISOs)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Println(InterruptSummary(steps, "okdctl destroy", tui.RunID()))
			return err
		}
		return err
	}

	duration := time.Since(startTime).Round(time.Second)
	tui.Info(fmt.Sprintf("cluster destroyed (%s)", duration))

	return nil
}

// runDestroyDryRun runs terraform plan -destroy so the operator can preview what
// would be removed. Returns *errtypes.ConfigError on plan failure so the process
// exits 2.
func runDestroyDryRun(ctx context.Context, cfg *config.Config) error {
	creds := handleCredentials(cfg)
	defer creds.Zeroize()

	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	tfEnv := phase.GetTerraformEnv(cfg)
	terraformDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", tfEnv)

	tfOpts := []terraform.Option{terraform.WithLogger(tui.SimpleLogger())}
	if creds.IsValid() {
		tfOpts = append(tfOpts, terraform.WithEnv(creds.Env()))
	}
	tf := terraform.New(terraformDir, tfOpts...)

	tui.Info(fmt.Sprintf("dry-run: terraform destroy plan for cluster '%s'", cfg.Cluster.Name))

	if err := tf.Init(ctx); err != nil {
		return &errtypes.ConfigError{Msg: "terraform init failed in dry-run", Err: err}
	}

	if err := tf.PlanStreamed(ctx, terraform.PlanOptions{Destroy: true}); err != nil {
		return &errtypes.ConfigError{Msg: "terraform destroy plan failed", Err: err}
	}

	tui.Info("dry-run: re-run without --dry-run to execute destroy")
	return nil
}
