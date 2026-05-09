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
	switch only {
	case "bootstrap":
		targets = []string{prefix + "bootstrap[0]"}
	case "masters":
		for i := range cfg.Topology.ControlPlane.Count {
			targets = append(targets, fmt.Sprintf("%smaster[%d]", prefix, i))
		}
	case "workers":
		for i := range cfg.Topology.Workers.Count {
			targets = append(targets, fmt.Sprintf("%sworker[%d]", prefix, i))
		}
	case "vms":
		targets = []string{prefix + "bootstrap[0]"}
		for i := range cfg.Topology.ControlPlane.Count {
			targets = append(targets, fmt.Sprintf("%smaster[%d]", prefix, i))
		}
		for i := range cfg.Topology.Workers.Count {
			targets = append(targets, fmt.Sprintf("%sworker[%d]", prefix, i))
		}
	default:
		return nil, &errtypes.ConfigError{
			Msg: fmt.Sprintf("--only %q is not valid; choose one of: vms, workers, masters, bootstrap", only),
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
		nodeType := m[1]
		bracket := m[2]
		if bracket == "" {
			continue
		}
		idx, _ := strconv.Atoi(bracket[1 : len(bracket)-1])
		switch nodeType {
		case "bootstrap":
			if idx != 0 {
				return &errtypes.UsageError{
					Msg: fmt.Sprintf("--target bootstrap index %d is out of range; bootstrap has exactly one node (index 0)", idx),
				}
			}
		case "master":
			if idx >= cfg.Topology.ControlPlane.Count {
				return &errtypes.UsageError{
					Msg: fmt.Sprintf("--target master[%d] is out of range; cluster has %d master(s) (valid: 0-%d)",
						idx, cfg.Topology.ControlPlane.Count, cfg.Topology.ControlPlane.Count-1),
				}
			}
		case "worker":
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
	Use:   "destroy",
	Short: "Destroy a Kubernetes cluster",
	Long: `Destroy a Kubernetes cluster and all associated infrastructure.
This operation is idempotent and safe to re-run if a previous destroy was interrupted.

Use --dry-run to preview the terraform destroy plan without modifying infra.

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
	destroyCmd.Flags().BoolVar(&destroyKeepISOs, "keep-isos", false, "do not remove the FCOS ISO from the Proxmox host")
	destroyCmd.Flags().BoolVar(&destroyDryRun, flagDryRun, false, "preview terraform destroy plan without running destroy")
	destroyCmd.Flags().StringVar(&destroyConfirmCluster, "confirm-cluster", "",
		"required with --yes; must equal cfg.Cluster.Name (typo guard for scripted destroys)")
	destroyCmd.Flags().BoolVar(&destroySkipTerraform, "skip-terraform", false, "skip terraform destroy — intended for resuming after a successful terraform-destroy phase (no-op with --dry-run)")
	destroyCmd.Flags().BoolVar(&destroySkipCleanup, "skip-cleanup", false, "skip host file cleanup — leaves haproxy/dnsmasq config in place (no-op with --dry-run)")
	destroyCmd.Flags().BoolVar(&destroySkipFirewall, "skip-firewall", false, "skip firewall rule cleanup (no-op with --dry-run)")
	destroyCmd.Flags().StringArrayVar(&destroyTargets, "target", nil,
		"limit terraform destroy to this resource address (repeatable); must match the okd_cluster VM allowlist")
	destroyCmd.Flags().StringVar(&destroyOnly, "only", "",
		"scope destroy to a node group: vms, workers, masters, bootstrap (expands into --target; mutually exclusive with --target)")
	destroyCmd.MarkFlagsMutuallyExclusive("only", "target")
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

	// --target/--only without --confirm-cluster lets a typo silently scope a
	// destroy; require an explicit cluster-name acknowledgement regardless of --yes.
	if len(destroyTargets) > 0 && destroyConfirmCluster == "" {
		return &errtypes.ConfigError{
			Msg: fmt.Sprintf("--target/--only requires --confirm-cluster=%q to guard against targeted destroys on the wrong cluster", cfg.Cluster.Name),
		}
	}

	if destroyDryRun {
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
			return &errtypes.ConfigError{
				Msg: fmt.Sprintf("%s cannot be used with --dry-run (dry-run only previews terraform; skip flags have no effect)",
					strings.Join(incompatible, ", ")),
			}
		}
		return runDestroyDryRun(ctx, cfg)
	}

	tui.Warn("this will destroy cluster and all associated resources", tui.LF("cluster", cfg.Cluster.Name))

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

	creds, err := handleCredentials(cfg)
	if err != nil {
		return err
	}
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
	defer p.ZeroizeEnv()

	announceDeployState(filepath.Join(workDir, deployStateFile))

	tui.Info("destroying cluster...")
	startTime := time.Now()

	steps, err := p.Destroy(ctx, cfg, okd.DestroyOpts{
		AutoApprove:      true,
		RemovePackages:   true,
		KeepISOs:         destroyKeepISOs,
		SkipTerraform:    destroySkipTerraform,
		SkipCleanup:      destroySkipCleanup,
		SkipFirewall:     destroySkipFirewall,
		TerraformTargets: destroyTargets,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(cmd.OutOrStdout(), InterruptSummary(steps, "okdctl destroy", tui.RunID()))
			return err
		}
		return err
	}

	duration := time.Since(startTime).Round(time.Second)
	tui.Info("cluster destroyed", tui.LF("duration", duration))

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

	tfEnv := phase.GetTerraformEnv(cfg)
	terraformDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", tfEnv)

	tfOpts := []terraform.Option{terraform.WithLogger(tui.SimpleLogger())}
	if creds.IsValid() {
		tfOpts = append(tfOpts, terraform.WithEnv(creds.Env()))
	}
	tf := terraform.New(terraformDir, tfOpts...)
	defer tf.ZeroizeEnv()

	tui.Info("dry-run: terraform destroy plan", tui.LF("cluster", cfg.Cluster.Name))

	if err := tf.Init(ctx); err != nil {
		if hint := tf.LockHint(); hint != nil {
			return errors.Join(hint, &errtypes.ConfigError{Msg: "terraform init failed in dry-run", Err: err})
		}
		return &errtypes.ConfigError{Msg: "terraform init failed in dry-run", Err: err}
	}

	if err := tf.PlanStreamed(ctx, terraform.PlanOptions{Destroy: true, Targets: destroyTargets}); err != nil {
		if hint := tf.LockHint(); hint != nil {
			return errors.Join(hint, &errtypes.ConfigError{Msg: "terraform destroy plan failed", Err: err})
		}
		return &errtypes.ConfigError{Msg: "terraform destroy plan failed", Err: err}
	}

	tui.Info("dry-run: re-run without --dry-run to execute destroy")
	return nil
}
