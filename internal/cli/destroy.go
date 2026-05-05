package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

var (
	destroyYes            bool
	destroyKeepISOs       bool
	destroyDryRun         bool
	destroyConfirmCluster string
	destroySkipTerraform  bool
	destroySkipCleanup    bool
	destroySkipFirewall   bool
	destroyTargets        []string
)

// destroyTargetRE matches valid terraform resource addresses for OKD VMs.
// Anchored to prevent partial matches that would silently widen scope.
var destroyTargetRE = regexp.MustCompile(
	`^module\.okd_cluster\.proxmox_virtual_environment_vm\.(bootstrap|master|worker)(\[\d+\])?$`,
)

func validateDestroyTargets(targets []string) error {
	for _, t := range targets {
		if !destroyTargetRE.MatchString(t) {
			return &errtypes.ConfigError{
				Msg: fmt.Sprintf("--target %q is not an allowed resource address; "+
					"must match module.okd_cluster.proxmox_virtual_environment_vm.{bootstrap|master|worker}[<n>]", t),
			}
		}
	}
	return nil
}

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
	destroyCmd.Flags().BoolVarP(&destroyYes, "yes", "y", false, "skip confirmation prompt")
	destroyCmd.Flags().BoolVar(&destroyKeepISOs, "keep-isos", false, "do not remove the FCOS ISO from the Proxmox host")
	destroyCmd.Flags().BoolVar(&destroyDryRun, flagDryRun, false, "preview terraform destroy plan without running destroy")
	destroyCmd.Flags().StringVar(&destroyConfirmCluster, "confirm-cluster", "",
		"required with --yes; must equal cfg.Cluster.Name (typo guard for scripted destroys)")
	destroyCmd.Flags().BoolVar(&destroySkipTerraform, "skip-terraform", false, "skip terraform destroy — intended for resuming after a successful terraform-destroy phase (no-op with --dry-run)")
	destroyCmd.Flags().BoolVar(&destroySkipCleanup, "skip-cleanup", false, "skip host file cleanup — leaves haproxy/dnsmasq config in place (no-op with --dry-run)")
	destroyCmd.Flags().BoolVar(&destroySkipFirewall, "skip-firewall", false, "skip firewall rule cleanup (no-op with --dry-run)")
	destroyCmd.Flags().StringArrayVar(&destroyTargets, "target", nil,
		"limit terraform destroy to this resource address (repeatable); must match the okd_cluster VM allowlist")
}

func runDestroy(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	tui.SetRunID(uuid.NewString())

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	if err := validateDestroyTargets(destroyTargets); err != nil {
		return err
	}

	// --target without --confirm-cluster lets a typo silently scope a destroy;
	// require an explicit cluster-name acknowledgement regardless of --yes.
	if len(destroyTargets) > 0 && destroyConfirmCluster == "" {
		return &errtypes.ConfigError{
			Msg: fmt.Sprintf("--target requires --confirm-cluster=%q to guard against targeted destroys on the wrong cluster", cfg.Cluster.Name),
		}
	}

	if destroyDryRun {
		return runDestroyDryRun(ctx, cfg)
	}

	tui.Warn(fmt.Sprintf("this will destroy cluster '%s' and all associated resources", cfg.Cluster.Name))

	if err := confirmClusterMatches(destroyYes, destroyConfirmCluster, cfg.Cluster.Name, "destroy"); err != nil {
		return err
	}

	if !destroyYes {
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

	lock, err := runlock.Acquire(projectRoot, "destroy")
	if err != nil {
		return err
	}
	defer lock.Release()

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

	steps, err := p.Destroy(ctx, cfg, okd.DestroyOpts{
		RemovePackages:   true,
		KeepISOs:         destroyKeepISOs,
		SkipTerraform:    destroySkipTerraform,
		SkipCleanup:      destroySkipCleanup,
		SkipFirewall:     destroySkipFirewall,
		TerraformTargets: destroyTargets,
	})
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

	lock, err := runlock.Acquire(projectRoot, "destroy --dry-run")
	if err != nil {
		return err
	}
	defer lock.Release()

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

	if err := tf.PlanStreamed(ctx, terraform.PlanOptions{Destroy: true, Targets: destroyTargets}); err != nil {
		return &errtypes.ConfigError{Msg: "terraform destroy plan failed", Err: err}
	}

	tui.Info("dry-run: re-run without --dry-run to execute destroy")
	return nil
}
