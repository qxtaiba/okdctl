package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/deploy"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/destroy"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

var (
	destroyYes            bool
	destroyKeepISOs       bool
	destroyDryRun         bool
	destroyConfirmCluster string
	destroySkipTerraform  bool
	destroySkipCleanup    bool
	destroySkipFirewall   bool
)

var destroyCmd = &cobra.Command{
	Use:   cmdNameDestroy,
	Short: "Destroy an OKD cluster",
	Long: `Destroy an OKD cluster and all associated infrastructure.
This operation is idempotent and safe to re-run if a previous destroy was interrupted.

Use --dry-run to preview the terraform destroy plan without modifying infra.
dry-run previews the terraform-destroy plan; the --skip-* flags resume a
partial terraform-destroy — the two address different failure points and
cannot be combined (see the --dry-run incompatibility check).

Master nodes ship with prevent_destroy = true in the Terraform module to
guard against accidental etcd-quorum loss. A fully-confirmed destroy
handles this automatically: after the confirmation gate passes, okdctl
writes a transient prevent_destroy_override.tf into
infrastructure/terraform/modules/proxmox-okd/ and removes it when the
destroy finishes (success or failure). Every non-destroy command refuses
to plan or apply while that file exists, so a stale copy from a crashed
run must be deleted by hand — the refusal names the path. If you must
override manually, an override file only merges within its own module, so
it belongs in the modules/proxmox-okd/ directory (never under
environments/). Alternatively, pass --skip-terraform to bypass Terraform
entirely and remove VMs by hand.`,
	Example: `  okdctl destroy                              # interactive prompt
  okdctl destroy --yes --confirm-cluster=prod # scripted destroy
  okdctl destroy --dry-run`,
	Args: cobra.NoArgs,
	RunE: runDestroy,
}

func init() {
	destroyCmd.Flags().BoolVarP(&destroyYes, "yes", "y", false, "skip confirmation prompt")
	destroyCmd.Flags().BoolVar(&destroyKeepISOs, "keep-isos", false, "do not remove the FCOS ISO from the Proxmox host")
	destroyCmd.Flags().BoolVar(&destroyDryRun, flagDryRun, false, "preview terraform destroy plan without running destroy")
	destroyCmd.Flags().StringVar(&destroyConfirmCluster, "confirm-cluster", "",
		"required with --yes; must equal the config cluster name")
	destroyCmd.Flags().BoolVar(&destroySkipTerraform, "skip-terraform", false, "skip terraform destroy — intended for resuming after a successful terraform-destroy phase (no-op with --dry-run)")
	destroyCmd.Flags().BoolVar(&destroySkipCleanup, "skip-cleanup", false, "skip host file cleanup — leaves haproxy/dnsmasq config in place (no-op with --dry-run)")
	destroyCmd.Flags().BoolVar(&destroySkipFirewall, "skip-firewall", false, "skip firewall rule cleanup (no-op with --dry-run)")
}

// validateDestroyFlagCombos rejects individually-valid but nonsensical flag
// combos; all exit 64 (EX_USAGE).
func validateDestroyFlagCombos(cfg *config.Config) error {
	// Checked even without --yes: a mismatched name means the operator is
	// pointed at the wrong cluster.
	if destroyConfirmCluster != "" && destroyConfirmCluster != cfg.Cluster.Name {
		return &errtypes.UsageError{
			Msg: fmt.Sprintf("--confirm-cluster %q does not match config cluster %q; refusing destroy",
				destroyConfirmCluster, cfg.Cluster.Name),
		}
	}
	if !destroyDryRun {
		return nil
	}
	var incompatible []string
	if destroySkipTerraform {
		incompatible = append(incompatible, "--skip-terraform")
	}
	if destroySkipCleanup {
		incompatible = append(incompatible, "--skip-cleanup")
	}
	if destroySkipFirewall {
		incompatible = append(incompatible, "--skip-firewall")
	}
	if len(incompatible) > 0 {
		return &errtypes.UsageError{
			Msg: fmt.Sprintf("%s cannot be used with --dry-run (dry-run only previews terraform; skip flags have no effect)",
				strings.Join(incompatible, ", ")),
		}
	}
	return nil
}

// confirmDestroyInteractive runs the two-stage gate: typed cluster name
// then y/N.
func confirmDestroyInteractive(ctx context.Context, cfg *config.Config) (bool, error) {
	nameConfirmed, err := promptForClusterNameConfirmation(ctx, cfg.Cluster.Name, tui.PromptLine("type cluster name to confirm destroy"))
	if err != nil || !nameConfirmed {
		return false, err
	}
	return promptForConfirmation(ctx, tui.PromptLine("proceed with destroy? [y/N]"))
}

// destroyAlsoRemovesFact reports the ConfirmBox "also removes" value: the
// non-terraform side effects this run will perform, given the skip-* flags.
func destroyAlsoRemovesFact() string {
	var removes []string
	if !destroyKeepISOs {
		removes = append(removes, "fcos iso")
	}
	if !destroySkipCleanup {
		removes = append(removes, "host files (work dir, haproxy/dnsmasq config, terraform state)")
	}
	if !destroySkipFirewall {
		removes = append(removes, "firewall rules")
	}
	if len(removes) == 0 {
		return "nothing (all skipped via flags)"
	}
	return strings.Join(removes, ", ")
}

// destroyConfirmFacts builds the ConfirmBox facts for cfg's cluster under
// the current skip-* flags.
func destroyConfirmFacts(cfg *config.Config) []render.Fact {
	return []render.Fact{
		{Key: factKeyCluster, Value: cfg.Cluster.Name},
		{Key: "domain", Value: cfg.Cluster.Domain},
		{Key: "scope", Value: "full cluster (all VMs)"},
		{Key: "also removes", Value: destroyAlsoRemovesFact()},
	}
}

func buildDestroyOptions(cfg *config.Config, projectRoot string) destroy.Options {
	opts := destroy.NewOptions(cfg, projectRoot)
	opts.AutoApprove = true
	opts.RemovePackages = true
	opts.KeepISOs = destroyKeepISOs
	opts.SkipTerraform = destroySkipTerraform
	opts.SkipCleanup = destroySkipCleanup
	opts.SkipFirewall = destroySkipFirewall
	return opts
}

// destroyNonTerraformActions lists the non-terraform side effects a destroy
// run performs, for the dry-run preview.
func destroyNonTerraformActions() []string {
	var actions []string
	if !destroyKeepISOs {
		actions = append(actions, "remove the FCOS ISO from the Proxmox host")
	}
	if !destroySkipCleanup {
		actions = append(actions, "run host cleanup (kind=full: work directory incl. kubeconfig and kubeadmin-password, haproxy/dnsmasq config, terraform state files, packages)")
	}
	if !destroySkipFirewall {
		actions = append(actions, "remove firewall rules")
	}
	if len(actions) == 0 {
		actions = append(actions, "nothing else (all skipped via flags)")
	}
	return actions
}

func runDestroy(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	if err := validateDestroyFlagCombos(cfg); err != nil {
		return err
	}

	if destroyDryRun {
		return runDestroyDryRun(ctx, cmd.OutOrStdout(), cfg)
	}

	// Resolved ahead of the confirmation gates so an in-flight node-op marker
	// is surfaced before the operator confirms.
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}
	announceInFlightNodeOp(projectRoot, cfg)

	fmt.Fprintln(cmd.ErrOrStderr(), render.ConfirmBox("destroy", destroyConfirmFacts(cfg), render.IrreversibleWarning))

	if err := confirmClusterMatches(destroyYes, destroyConfirmCluster, cfg.Cluster.Name, "destroy"); err != nil {
		return err
	}

	if !destroyYes {
		proceed, err := confirmDestroyInteractive(ctx, cfg)
		if err != nil {
			return err
		}
		if !proceed {
			logutil.Info("cancelled")
			return nil
		}
	}

	creds, err := handleCredentials(cfg)
	if err != nil {
		return err
	}
	defer creds.Zeroize()

	lock, err := runlock.Acquire(projectRoot, "destroy")
	if err != nil {
		return err
	}
	defer lock.Release()

	// A partial/cancelled run may leave the workdir root-owned; restore
	// invoking-user ownership at exit.
	workDir := workspace.WorkDir(projectRoot)
	defer func() {
		if chownErr := system.ChownTreeToInvokingUser(workDir); chownErr != nil {
			logutil.Warn("workdir chown back to user incomplete", logutil.LF("err", chownErr))
		}
	}()

	p := deploy.NewProvisioner(creds, projectRoot)
	defer p.ZeroizeEnv()

	deploy.AnnounceState(filepath.Join(workDir, deploy.StateFileName), cfg.Cluster.Name)

	logutil.Info("destroying cluster...")
	startTime := time.Now()

	destroyOpts := buildDestroyOptions(cfg, projectRoot)

	steps, err := p.Destroy(ctx, cfg, &destroyOpts)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(cmd.ErrOrStderr(), render.InterruptSummary(steps, "okdctl destroy", logutil.RunID()))
			return render.Presented(err)
		}
		return err
	}

	duration := time.Since(startTime).Round(time.Second)
	logutil.Info("cluster destroyed", logutil.LF("duration", duration))

	return nil
}

// runDestroyDryRun runs terraform plan -destroy to preview removal, then
// prints the boxed non-terraform preview to w; a plan failure returns
// *errtypes.ConfigError (exit 2).
func runDestroyDryRun(ctx context.Context, w io.Writer, cfg *config.Config) error {
	creds, err := handleCredentials(cfg)
	if err != nil {
		return err
	}
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

	tfEnv := cfg.TerraformEnvName()
	terraformDir := workspace.TerraformEnvDir(projectRoot, tfEnv)

	tfOpts := []terraform.Option{terraform.WithLogger(logutil.SimpleLogger())}
	if creds.IsValid() {
		tfOpts = append(tfOpts, terraform.WithEnv(creds.Env()))
	}
	tf := terraform.New(terraformDir, tfOpts...)
	defer tf.ZeroizeEnv()

	if err := tf.Init(ctx); err != nil {
		return tf.WithLockHint(&errtypes.ConfigError{Msg: "terraform init failed in dry-run", Err: err})
	}

	if err := tf.PreviewDestroy(ctx, workspace.TerraformModuleDir(projectRoot)); err != nil {
		return tf.WithLockHint(&errtypes.ConfigError{Msg: "terraform destroy plan failed", Err: err})
	}

	fmt.Fprintln(w, render.DryRunActions("destroy", destroyConfirmFacts(cfg), destroyNonTerraformActions()))
	return nil
}
