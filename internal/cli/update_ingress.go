package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/deploy"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/dns"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

var (
	updateIngressYes            bool
	updateIngressKeepHAProxy    bool
	updateIngressDryRun         bool
	updateIngressConfirmCluster string
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
	updateIngressCmd.Flags().BoolVar(&updateIngressDryRun, flagDryRun, false, "preview update-ingress mutations without touching the cluster")
	updateIngressCmd.Flags().StringVar(&updateIngressConfirmCluster, "confirm-cluster", "",
		"required with --yes; must equal cfg.Cluster.Name (typo guard for scripted update-ingress runs)")
}

// runUpdateIngressDryRun prints the mutations update-ingress would perform
// without connecting to the cluster or modifying any host configuration.
// It probes on-disk dnsmasq state and the haproxy service to label any
// steps that would already be no-ops on the live system.
func runUpdateIngressDryRun(ctx context.Context, cfg *config.Config) error {
	clusterFQDN := cfg.Cluster.Name + "." + cfg.Cluster.Domain
	tui.Info("dry-run: update-ingress for cluster", tui.LF("cluster", clusterFQDN))
	tui.Info("would: query IngressControllers (oc get ingresscontroller -n openshift-ingress-operator)")
	tui.Info("would: wait for LoadBalancer IPs on router-* services in openshift-ingress")

	isBootstrap, err := dns.IsBootstrapDNS(cfg)
	if err != nil {
		return fmt.Errorf("dry-run: failed to probe dnsmasq state: %w", err)
	}
	if isBootstrap {
		tui.Info("would: deploy production dnsmasq config pointing *.apps at LoadBalancer IPs")
	} else {
		tui.Info("would: deploy production dnsmasq config pointing *.apps at LoadBalancer IPs (no-op: dns already cut over)")
	}

	if !updateIngressKeepHAProxy {
		if system.IsServiceActive(ctx, "haproxy") {
			tui.Info("would: stop and disable haproxy on the bastion (if all controllers are LB-type)")
		} else {
			tui.Info("would: stop and disable haproxy on the bastion (no-op: haproxy already stopped)")
		}
	}
	tui.Info("dry-run: re-run without --dry-run to execute update-ingress")
	return nil
}

func buildConvertConfirm(ctx context.Context, yes bool) func([]string) bool {
	return func(hostNetworkICs []string) bool {
		tui.Warn("converting HostNetwork controller(s) to LoadBalancerService requires deleting and recreating them", tui.LF("count", len(hostNetworkICs)))
		tui.Warn("this will cause a brief outage (~30s) for routes on affected controllers.")

		if yes {
			return true
		}

		prompt := fmt.Sprintf("convert %d HostNetwork controller(s) to LoadBalancerService? [y/N]: ", len(hostNetworkICs))
		confirmed, err := promptForConfirmation(ctx, prompt)
		if err != nil {
			tui.Warn("skipping HostNetwork conversion", tui.LF("err", err))
			return false
		}
		return confirmed
	}
}

func runUpdateIngress(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	if updateIngressDryRun {
		return runUpdateIngressDryRun(ctx, cfg)
	}

	if err := confirmClusterMatches(updateIngressYes, updateIngressConfirmCluster, cfg.Cluster.Name, "update-ingress"); err != nil {
		return err
	}

	clusterFQDN := cfg.Cluster.Name + "." + cfg.Cluster.Domain
	tui.Warn("this will update dns to use loadbalancer ips", tui.LF("cluster", clusterFQDN))
	if !updateIngressKeepHAProxy {
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

	p := deploy.NewProvisioner(nil, projectRoot)

	tui.Info("detecting ingress strategy and loadbalancer ips...")
	startTime := time.Now()

	// WorkDir stays empty so the provisioner defaults it to
	// <projectRoot>/okd-install; passing projectRoot here pointed
	// RemoveHAProxy at a cluster-config path that never exists.
	result, err := p.UpdateIngress(ctx, cfg, postinstall.UpdateIngressOptions{
		RemoveHAProxy:     !updateIngressKeepHAProxy,
		ConfirmConversion: buildConvertConfirm(ctx, updateIngressYes),
	})
	if err != nil {
		return err
	}

	duration := time.Since(startTime).Round(time.Second)
	tui.Info("ingress updated", tui.LF("duration", duration))
	fmt.Fprintln(cmd.OutOrStdout(), render.UpdateIngressSummary(result))

	return nil
}
