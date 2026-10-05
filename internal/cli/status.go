package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/deploy"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/clusterstatus"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

var statusOutput string

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print a post-deploy cluster summary",
	Long: `Print API reachability, node counts by role, cluster operator
health, and addon status for the deployed cluster.`,
	Example: `  okdctl status
  okdctl status --output json | jq '.nodes'
  okdctl status --output json | jq '[.nodes[] | select(.ready)] | length'`,
	Args: cobra.NoArgs,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().StringVarP(&statusOutput, flagOutput, flagOutputShort, outputText, "output format: text|json")
	registerOutputCompletion(statusCmd)
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, _ []string) error {
	if err := validateFormat(statusOutput); err != nil {
		return err
	}
	quietForJSON(statusOutput)

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	var cl clusterstatus.Client
	if c, clErr := clusterstatus.NewClient(projectRoot); clErr == nil {
		cl = c
	}

	src, cleanup := statusLifecycleSources(cfg, projectRoot)
	defer cleanup()

	cs := clusterstatus.Collect(cmd.Context(), cl, newAddonManager(cfg, projectRoot), src)

	if statusOutput == outputJSON {
		return writeJSON(cmd.OutOrStdout(), cs)
	}
	return printClusterStatus(cmd, &cs)
}

func validateFormat(format string) error {
	switch format {
	case outputText, outputJSON:
		return nil
	default:
		return &errtypes.UsageError{Msg: fmt.Sprintf("invalid --output %q (want text|json)", format)}
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// statusLifecycleSources wires phase-derivation signals; credentials load
// silently (status degrades gracefully without them) and the returned cleanup
// zeroizes them.
func statusLifecycleSources(cfg *config.Config, projectRoot string) (src clusterstatus.LifecycleSources, cleanup func()) {
	src = clusterstatus.LifecycleSources{
		DeployInProgress: func() bool {
			return deploy.InstallInProgress(workspace.WorkDir(projectRoot), cfg.Cluster.Name)
		},
		InfraPresent: func() bool {
			return clusterstatus.TerraformStateHasResources(projectRoot, cfg.TerraformEnvName())
		},
	}
	if err := credentials.LoadEnvFile(credentials.EnvFilePath(cfgFile)); err != nil {
		return src, func() {}
	}
	creds := credentials.GetProxmoxCredentials(cfg)
	if !creds.IsValid() {
		creds.Zeroize()
		return src, func() {}
	}
	src.Power = &proxmoxPowerProber{cfg: cfg, creds: creds}
	return src, creds.Zeroize
}

// proxmoxPowerProber adapts proxmox.VMPowerStates to the clusterstatus seam,
// covering masters+workers (bootstrap is covered separately by the deploy
// marker).
type proxmoxPowerProber struct {
	cfg   *config.Config
	creds *credentials.ProxmoxCredentials
}

func (p *proxmoxPowerProber) VMStates(ctx context.Context) (map[int]nodetypes.VMState, error) {
	var vmids []int
	for i := range p.cfg.Topology.ControlPlane.Count {
		vmids = append(vmids, nodetypes.VMID(p.cfg, nodetypes.RoleMaster, i))
	}
	for i := range p.cfg.Topology.Workers.Count {
		vmids = append(vmids, nodetypes.VMID(p.cfg, nodetypes.RoleWorker, i))
	}
	return proxmox.VMPowerStates(ctx, &proxmox.ProbeOptions{
		Endpoint: p.creds.Endpoint,
		Username: p.creds.Username,
		Password: p.creds.Password,
		APIToken: p.creds.APIToken,
		Insecure: p.creds.Insecure,
	}, vmids)
}

func printClusterStatus(cmd *cobra.Command, st *okd.ClusterStatus) error {
	_, err := fmt.Fprintln(cmd.OutOrStdout(), render.ClusterStatusBox(st))
	return err
}
