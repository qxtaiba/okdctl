package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/deploy"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/destroy"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

type destroyScope string

const (
	scopeBootstrap destroyScope = "bootstrap"
	scopeMasters   destroyScope = "masters"
	scopeWorkers   destroyScope = "workers"
	scopeVMs       destroyScope = "vms"
)

func validDestroyScopes() []string {
	return []string{string(scopeVMs), string(scopeWorkers), string(scopeMasters), string(scopeBootstrap)}
}

var (
	destroyYes            bool
	destroyKeepISOs       bool
	destroyDryRun         bool
	destroyConfirmCluster string
	destroySkipTerraform  bool
	destroySkipCleanup    bool
	destroySkipFirewall   bool
	destroyTargets        []string
	destroyOnly           string
)

// destroyTargetRE matches valid terraform resource addresses for OKD VMs.
// Anchored to prevent partial matches that would silently widen scope.
var destroyTargetRE = regexp.MustCompile(
	`^module\.okd_cluster\.proxmox_virtual_environment_vm\.(bootstrap|master|worker)(\[\d+\])?$`,
)

// expandOnlyFlag converts --only into the equivalent --target list for the
// given cluster topology. Workers and masters use count-indexed addresses;
// bootstrap is always index 0.
func expandOnlyFlag(only string, cfg *config.Config) ([]string, error) {
	const prefix = "module.okd_cluster.proxmox_virtual_environment_vm."
	var targets []string
	switch destroyScope(only) {
	case scopeBootstrap:
		targets = []string{prefix + "bootstrap[0]"}
	case scopeMasters:
		for i := range cfg.Topology.ControlPlane.Count {
			targets = append(targets, fmt.Sprintf("%smaster[%d]", prefix, i))
		}
	case scopeWorkers:
		for i := range cfg.Topology.Workers.Count {
			targets = append(targets, fmt.Sprintf("%sworker[%d]", prefix, i))
		}
	case scopeVMs:
		targets = []string{prefix + "bootstrap[0]"}
		for i := range cfg.Topology.ControlPlane.Count {
			targets = append(targets, fmt.Sprintf("%smaster[%d]", prefix, i))
		}
		for i := range cfg.Topology.Workers.Count {
			targets = append(targets, fmt.Sprintf("%sworker[%d]", prefix, i))
		}
	default:
		return nil, &errtypes.ConfigError{
			Msg: fmt.Sprintf("--only %q is not valid; choose one of: %s", only, strings.Join(validDestroyScopes(), ", ")),
		}
	}
	if len(targets) == 0 {
		return nil, &errtypes.ConfigError{
			Msg: fmt.Sprintf("--only=%s produced no targets; check topology counts in config", only),
		}
	}
	return targets, nil
}

func validateDestroyTargets(targets []string, cfg *config.Config) error {
	for _, t := range targets {
		m := destroyTargetRE.FindStringSubmatch(t)
		if m == nil {
			return &errtypes.ConfigError{
				Msg: fmt.Sprintf("--target %q is not an allowed resource address; "+
					"must match module.okd_cluster.proxmox_virtual_environment_vm.{bootstrap|master|worker}[<n>]", t),
			}
		}
		role, roleErr := nodetypes.ParseNodeRole(m[1])
		if roleErr != nil {
			// destroyTargetRE already restricts m[1] to bootstrap|master|worker.
			continue
		}
		bracket := m[2]
		if bracket == "" {
			continue
		}
		idx, _ := strconv.Atoi(bracket[1 : len(bracket)-1])
		switch role {
		case nodetypes.RoleBootstrap:
			if idx != 0 {
				return &errtypes.UsageError{
					Msg: fmt.Sprintf("--target bootstrap index %d is out of range; bootstrap has exactly one node (index 0)", idx),
				}
			}
		case nodetypes.RoleMaster:
			if idx >= cfg.Topology.ControlPlane.Count {
				return &errtypes.UsageError{
					Msg: fmt.Sprintf("--target master[%d] is out of range; cluster has %d master(s) (valid: 0-%d)",
						idx, cfg.Topology.ControlPlane.Count, cfg.Topology.ControlPlane.Count-1),
				}
			}
		case nodetypes.RoleWorker:
			if idx >= cfg.Topology.Workers.Count {
				return &errtypes.UsageError{
					Msg: fmt.Sprintf("--target worker[%d] is out of range; cluster has %d worker(s) (valid: 0-%d)",
						idx, cfg.Topology.Workers.Count, cfg.Topology.Workers.Count-1),
				}
			}
		}
	}
	return nil
}

var destroyCmd = &cobra.Command{
	Use:   cmdNameDestroy,
	Short: "Destroy a Kubernetes cluster",
	Long: `Destroy a Kubernetes cluster and all associated infrastructure.
This operation is idempotent and safe to re-run if a previous destroy was interrupted.

Use --dry-run to preview the terraform destroy plan without modifying infra.
dry-run previews the terraform-destroy plan; the --skip-* flags resume a
partial terraform-destroy — the two address different failure points and
cannot be combined (see the --dry-run incompatibility check).

A scoped destroy (--target or --only) only tears down the named Terraform
resources; host cleanup (haproxy/dnsmasq config, kubeconfig, terraform state
files), firewall rules, and Proxmox ISO removal are skipped automatically for
a scoped run — that bastion-wide teardown runs only on an unscoped destroy,
so it never touches a still-running control plane.

Master nodes ship with prevent_destroy = true in the Terraform module to
guard against accidental etcd-quorum loss. To run a full or targeted
destroy, place an override.tf in
infrastructure/terraform/modules/proxmox-okd/ disabling prevent_destroy on
the master resource:

  resource "proxmox_virtual_environment_vm" "master" {
    lifecycle {
      prevent_destroy = false
    }
  }

Remove the override.tf after destroy completes. Alternatively, pass
--skip-terraform to bypass Terraform entirely and remove VMs by hand.`,
	Example: `  okdctl destroy                              # interactive prompt
  okdctl destroy --yes --confirm-cluster=prod # scripted destroy
  okdctl destroy --dry-run`,
	RunE: runDestroy,
}

func init() {
	destroyCmd.Flags().BoolVarP(&destroyYes, "yes", "y", false, "skip confirmation prompt")
	destroyCmd.Flags().BoolVar(&destroyKeepISOs, "keep-isos", false, "do not remove the FCOS ISO from the Proxmox host (always true for a scoped --target/--only destroy)")
	destroyCmd.Flags().BoolVar(&destroyDryRun, flagDryRun, false, "preview terraform destroy plan without running destroy")
	destroyCmd.Flags().StringVar(&destroyConfirmCluster, "confirm-cluster", "",
		"required with --yes; must equal the config cluster name")
	destroyCmd.Flags().BoolVar(&destroySkipTerraform, "skip-terraform", false, "skip terraform destroy — intended for resuming after a successful terraform-destroy phase (no-op with --dry-run)")
	destroyCmd.Flags().BoolVar(&destroySkipCleanup, "skip-cleanup", false, "skip host file cleanup — leaves haproxy/dnsmasq config in place (no-op with --dry-run; always true for a scoped --target/--only destroy)")
	destroyCmd.Flags().BoolVar(&destroySkipFirewall, "skip-firewall", false, "skip firewall rule cleanup (no-op with --dry-run; always true for a scoped --target/--only destroy)")
	destroyCmd.Flags().StringArrayVar(&destroyTargets, flagTarget, nil,
		"limit terraform destroy to this resource address (repeatable); must match the okd_cluster VM allowlist; scopes cleanup/firewall/iso removal off automatically")
	destroyCmd.Flags().StringVar(&destroyOnly, flagOnly, "",
		"scope destroy to a node group: "+strings.Join(validDestroyScopes(), ", ")+" (expands into --target; mutually exclusive with --target; scopes cleanup/firewall/iso removal off automatically)")
	destroyCmd.MarkFlagsMutuallyExclusive(flagOnly, flagTarget)
	_ = destroyCmd.RegisterFlagCompletionFunc(flagOnly, func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return validDestroyScopes(), cobra.ShellCompDirectiveNoFileComp
	})
}

// validateDestroyFlagCombos rejects flag combinations that are individually
// valid but not sensible together; all exit 64 (EX_USAGE).
func validateDestroyFlagCombos(cfg *config.Config) error {
	// --target/--only without --confirm-cluster lets a typo silently scope a
	// destroy; require an explicit cluster-name acknowledgement regardless of --yes.
	if len(destroyTargets) > 0 && destroyConfirmCluster == "" {
		return &errtypes.UsageError{
			Msg: fmt.Sprintf("--target/--only requires --confirm-cluster=%q to guard against targeted destroys on the wrong cluster", cfg.Cluster.Name),
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

// confirmDestroyInteractive runs the two-stage interactive gate: exact
// cluster-name typing (unscoped runs only — scoped runs already passed
// --confirm-cluster) followed by the y/N prompt.
func confirmDestroyInteractive(ctx context.Context, cfg *config.Config) (bool, error) {
	if len(destroyTargets) == 0 {
		nameConfirmed, err := promptForClusterNameConfirmation(ctx, cfg.Cluster.Name, "type cluster name to confirm destroy: ")
		if err != nil || !nameConfirmed {
			return false, err
		}
	}
	return promptForConfirmation(ctx, "proceed with destroy? [y/N]: ")
}

// buildDestroyOptions assembles destroy.Options from the destroy flag set.
// A scoped run (--target/--only) forces the cleanup/firewall/iso steps off
// so bastion-wide teardown never runs against a still-running control plane.
func buildDestroyOptions(cfg *config.Config, projectRoot string) destroy.Options {
	skipCleanup := destroySkipCleanup
	skipFirewall := destroySkipFirewall
	keepISOs := destroyKeepISOs
	if len(destroyTargets) > 0 {
		skipCleanup = true
		skipFirewall = true
		keepISOs = true
		logutil.Info("scoped destroy: skipping host cleanup, firewall rules, and iso removal — full bastion teardown is exclusive to an unscoped destroy")
	}

	opts := destroy.NewOptions(cfg, projectRoot)
	opts.AutoApprove = true
	opts.RemovePackages = true
	opts.KeepISOs = keepISOs
	opts.SkipTerraform = destroySkipTerraform
	opts.SkipCleanup = skipCleanup
	opts.SkipFirewall = skipFirewall
	opts.TerraformTargets = destroyTargets
	return opts
}

func runDestroy(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	if destroyOnly != "" {
		expanded, err := expandOnlyFlag(destroyOnly, cfg)
		if err != nil {
			return err
		}
		destroyTargets = expanded
	}

	if err := validateDestroyTargets(destroyTargets, cfg); err != nil {
		return err
	}

	if err := validateDestroyFlagCombos(cfg); err != nil {
		return err
	}

	if destroyDryRun {
		return runDestroyDryRun(ctx, cfg)
	}

	logutil.Warn("this will destroy cluster and all associated resources", logutil.LF("cluster", cfg.Cluster.Name))

	// Resolved ahead of the confirmation gates so an in-flight node-op marker
	// is surfaced before the operator confirms (with --yes it still lands in
	// the log as a warning). Destroy proceeds either way — the teardown covers
	// whatever partial work the interrupted op left in terraform state.
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}
	announceInFlightNodeOp(projectRoot, cfg)

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

	// Destroy writes .tfstate + logs into <projectRoot>/okd-install before
	// tearing down. On partial/cancelled runs the workdir may survive
	// root-owned; restore invoking-user ownership at exit so the user can
	// inspect or retry.
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
			fmt.Fprintln(cmd.OutOrStdout(), render.InterruptSummary(steps, "okdctl destroy", logutil.RunID()))
			return render.Presented(err)
		}
		return err
	}

	duration := time.Since(startTime).Round(time.Second)
	logutil.Info("cluster destroyed", logutil.LF("duration", duration))

	return nil
}

// runDestroyDryRun runs terraform plan -destroy so the operator can preview what
// would be removed. Returns *errtypes.ConfigError on plan failure so the process
// exits 2.
func runDestroyDryRun(ctx context.Context, cfg *config.Config) error {
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

	logutil.Info("dry-run: terraform destroy plan", logutil.LF("cluster", cfg.Cluster.Name))

	if err := tf.Init(ctx); err != nil {
		return tf.WithLockHint(&errtypes.ConfigError{Msg: "terraform init failed in dry-run", Err: err})
	}

	if err := tf.PlanStreamed(ctx, terraform.PlanOptions{Destroy: true, Targets: destroyTargets}); err != nil {
		return tf.WithLockHint(&errtypes.ConfigError{Msg: "terraform destroy plan failed", Err: err})
	}

	logutil.Info("dry-run: re-run without --dry-run to execute destroy")
	return nil
}
