package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
)

var (
	updateIngressYes           bool
	updateIngressRemoveHAProxy bool
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
	RunE: runUpdateIngress,
}

func init() {
	updateIngressCmd.Flags().BoolVarP(&updateIngressYes, "yes", "y", false, "skip confirmation prompts")
	updateIngressCmd.Flags().BoolVar(&updateIngressRemoveHAProxy, "remove-haproxy", true, "remove haproxy from bastion after dns switch")
}

func runUpdateIngress(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		return err
	}

	clusterFQDN := cfg.Cluster.Name + "." + cfg.Cluster.Domain
	tui.Warn("this will update dns for '" + clusterFQDN + "' to use loadbalancer ips")
	if updateIngressRemoveHAProxy {
		tui.Warn("haproxy will be stopped and disabled on the bastion")
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

	p := CreateOKDProvisionerNoCreds(cfg)

	tui.Info("detecting ingress strategy and loadbalancer ips...")
	startTime := time.Now()

	result, err := p.UpdateIngress(ctx, cfg, &okd.UpdateIngressOptions{
		RemoveHAProxy: updateIngressRemoveHAProxy,
		ConfirmConversion: func(hostNetworkICs []string) bool {
			tui.Warn(fmt.Sprintf("converting %d HostNetwork controller(s) to LoadBalancerService requires deleting and recreating them.", len(hostNetworkICs)))
			tui.Warn("this will cause a brief outage (~30s) for routes on affected controllers.")

			if updateIngressYes {
				return true
			}

			prompt := fmt.Sprintf("convert %d HostNetwork controller(s) to LoadBalancerService? [y/N]: ", len(hostNetworkICs))
			confirmed, err := promptForConfirmation(ctx, prompt)
			if err != nil {
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
