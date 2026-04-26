package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/tui"
)

var (
	updateIngressYes           bool
	updateIngressRemoveHAProxy bool
	updateIngressKeepHAProxy   bool
	updateIngressDryRun        bool
)

var updateIngressCmd = &cobra.Command{
	Use:   "update-ingress",
	Short: "Switch ingress DNS from HAProxy to LoadBalancer IPs",
	Long: `Detect IngressController strategies and LoadBalancer IPs, then update
DNS records to point *.apps at the real LoadBalancer IP instead of
the bastion HAProxy.

If any IngressControllers use HostNetwork (common on bare-metal OKD)
and MetalLB is available, you will be prompted to convert them to
LoadBalancerService. This requires deleting and recreating the
IngressController, which causes a brief outage (~30s) for routes on
affected controllers.

Run this after deploying a LoadBalancer provider (e.g., MetalLB).`,
	Example: `  okdctl update-ingress
  okdctl update-ingress --yes --keep-haproxy
  okdctl update-ingress --dry-run`,
	RunE: runUpdateIngress,
}

func init() {
	updateIngressCmd.Flags().BoolVarP(&updateIngressYes, "yes", "y", false, "skip confirmation prompts")
	updateIngressCmd.Flags().BoolVar(&updateIngressKeepHAProxy, "keep-haproxy", false, "keep haproxy running on the bastion after dns switch")
	updateIngressCmd.Flags().BoolVar(&updateIngressRemoveHAProxy, "remove-haproxy", true, "deprecated: use --keep-haproxy instead")
	if err := updateIngressCmd.Flags().MarkDeprecated("remove-haproxy", "use --keep-haproxy instead"); err != nil {
		panic(err) // flag is statically defined above; unreachable
	}
	updateIngressCmd.Flags().BoolVar(&updateIngressDryRun, "dry-run", false, "preview update-ingress mutations without touching the cluster")
}

// runUpdateIngressDryRun prints the mutations update-ingress would perform
// without connecting to the cluster or modifying any host configuration.
func runUpdateIngressDryRun(cfg *config.Config) error { //nolint:unparam // error return preserved for symmetry with non-dry-run path
	clusterFQDN := cfg.Cluster.Name + "." + cfg.Cluster.Domain
	tui.Info(fmt.Sprintf("dry-run: update-ingress for cluster '%s'", clusterFQDN))
	fmt.Println("  would: query IngressControllers (oc get ingresscontroller -n openshift-ingress-operator)")
	fmt.Println("  would: wait for LoadBalancer IPs on router-* services in openshift-ingress")
	fmt.Println("  would: deploy production dnsmasq config pointing *.apps at LoadBalancer IPs")
	if updateIngressRemoveHAProxy && !updateIngressKeepHAProxy {
		fmt.Println("  would: stop and disable haproxy on the bastion (if all controllers are LB-type)")
	}
	tui.Info("dry-run: re-run without --dry-run to execute update-ingress")
	return nil
}

func runUpdateIngress(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	if updateIngressDryRun {
		return runUpdateIngressDryRun(cfg)
	}

	clusterFQDN := cfg.Cluster.Name + "." + cfg.Cluster.Domain
	tui.Warn(fmt.Sprintf("this will update dns for '%s' to use loadbalancer ips", clusterFQDN))
	effectiveRemoveHAProxy := updateIngressRemoveHAProxy && !updateIngressKeepHAProxy
	if effectiveRemoveHAProxy {
		tui.Warn("haproxy will be stopped and disabled on the bastion (pass --keep-haproxy to skip)")
	}

	if !updateIngressYes {
		confirmed, err := promptForConfirmation(ctx, "proceed with ingress update? [y/N]: ")
		if err != nil {
			return err
		}
		if !confirmed {
			tui.Info("cancelled")
			return nil
		}
	}

	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	lock, err := runlock.Acquire(projectRoot, "update-ingress")
	if err != nil {
		return err
	}
	defer lock.Release()

	p := createOKDProvisionerWithOpts(cfg, nil, projectRoot)

	tui.Info("detecting ingress strategy and loadbalancer ips...")
	startTime := time.Now()

	result, err := p.UpdateIngress(ctx, cfg, postinstall.UpdateIngressOptions{
		RemoveHAProxy: effectiveRemoveHAProxy,
		ConfirmConversion: func(hostNetworkICs []string) bool {
			tui.Warn(fmt.Sprintf("converting %d HostNetwork controller(s) to LoadBalancerService requires deleting and recreating them.", len(hostNetworkICs)))
			tui.Warn("this will cause a brief outage (~30s) for routes on affected controllers.")

			if updateIngressYes {
				return true
			}

			prompt := fmt.Sprintf("convert %d HostNetwork controller(s) to LoadBalancerService? [y/N]: ", len(hostNetworkICs))
			confirmed, err := promptForConfirmation(ctx, prompt)
			if err != nil {
				tui.Warn("skipping HostNetwork conversion: " + err.Error())
				return false
			}
			return confirmed
		},
	})
	if err != nil {
		return err
	}

	duration := time.Since(startTime).Round(time.Second)
	tui.Info(fmt.Sprintf("ingress updated (%s)", duration))
	fmt.Println(UpdateIngressSummary(result))

	return nil
}
