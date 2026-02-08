package cli

import (
	"context"
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
	Long: `Detect LoadBalancer IPs assigned to the ingress routers and update
DNS records to point *.apps at the real LoadBalancer IP instead of
the bastion HAProxy.

Run this after deploying a LoadBalancer provider (e.g., MetalLB)
and confirming that router-default has an external IP.`,
	RunE: runUpdateIngress,
}

func init() {
	updateIngressCmd.Flags().BoolVarP(&updateIngressYes, "yes", "y", false, "skip confirmation prompt")
	updateIngressCmd.Flags().BoolVar(&updateIngressRemoveHAProxy, "remove-haproxy", true, "remove haproxy from bastion after dns switch")
}

func runUpdateIngress(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

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

	tui.Info("detecting loadbalancer ips...")
	startTime := time.Now()

	result, err := p.UpdateIngress(ctx, cfg, &okd.UpdateIngressOptions{
		RemoveHAProxy: updateIngressRemoveHAProxy,
	})
	if err != nil {
		return err
	}

	duration := time.Since(startTime).Round(time.Second)
	tui.Info(fmt.Sprintf("ingress updated (%s)", duration))
	fmt.Println(UpdateIngressSummary(cfg, result))

	return nil
}
