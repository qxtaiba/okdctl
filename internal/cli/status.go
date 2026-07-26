package cli

import (
	"fmt"
	"strconv"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/clusterstatus"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
)

const colName = "name"

var statusOutput string

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print a post-deploy cluster summary",
	Long: `Print API reachability, node counts by role, cluster operator
health, and addon status for the deployed cluster.`,
	Example: `  okdctl status
  okdctl status --output json | jq '.nodes'
  okdctl status --output json | jq '[.nodes[] | select(.ready)] | length'`,
	RunE: runStatus,
}

var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Show details for a cluster node or addon",
	Long:  "Show detailed information for a specific cluster node or registered addon.",
}

var describeNodeCmd = &cobra.Command{
	Use:   "node <name>",
	Short: "Show detail for a cluster node",
	Long: `Show the name, role (master/worker), and readiness condition for a
single cluster node retrieved via oc. Use 'okdctl node list' to see every
node at once (with terraform index and sizing-drift), or 'okdctl status' for
a cluster-wide summary.`,
	Example: "  okdctl describe node master-0",
	Args:    cobra.ExactArgs(1),
	RunE:    runDescribeNode,
}

var describeAddonCmd = &cobra.Command{
	Use:   "addon <name>",
	Short: "Show detail for a registered addon",
	Long: `Show metadata (display name, description, category) and live health for
a registered addon by running its Verify() probe against the cluster.
Use 'okdctl addon list' to see all available addon names.`,
	Example: "  okdctl describe addon flux",
	Args:    cobra.ExactArgs(1),
	RunE:    runDescribeAddon,
	ValidArgsFunction: func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return addon.Names(), cobra.ShellCompDirectiveNoFileComp
	},
}

var (
	describeNodeOutput  string
	describeAddonOutput string
)

func init() {
	statusCmd.Flags().StringVarP(&statusOutput, flagOutput, flagOutputShort, outputText, "output format: text|json")
	registerOutputCompletion(statusCmd)
	describeNodeCmd.Flags().StringVarP(&describeNodeOutput, flagOutput, flagOutputShort, outputText, "output format: text|json")
	registerOutputCompletion(describeNodeCmd)
	describeAddonCmd.Flags().StringVarP(&describeAddonOutput, flagOutput, flagOutputShort, outputText, "output format: text|json")
	registerOutputCompletion(describeAddonCmd)
	describeCmd.AddCommand(describeNodeCmd)
	describeCmd.AddCommand(describeAddonCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(describeCmd)
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

	cs := clusterstatus.Collect(cmd.Context(), cl, newAddonManager(cfg, projectRoot), projectRoot)

	if statusOutput == outputJSON {
		return writeJSON(cmd.OutOrStdout(), cs)
	}
	return printClusterStatus(cmd, &cs)
}

func printClusterStatus(cmd *cobra.Command, st *okd.ClusterStatus) error {
	sb := render.NewBuilder()
	sb.WriteString("\n")

	sb.Section("cluster")
	sb.KV("phase", string(st.Phase))
	sb.Newline()

	sb.Section("api")
	if st.APIReachable {
		sb.KV("reachable", "yes")
	} else {
		sb.KV("reachable", "no (oc get --raw /healthz failed)")
	}
	sb.Newline()

	masters := 0
	workers := 0
	for _, n := range st.Nodes {
		switch n.Role {
		case nodetypes.RoleMaster:
			masters++
		case nodetypes.RoleWorker:
			workers++
		}
	}
	sb.Section("nodes")
	if len(st.Nodes) == 0 {
		sb.WriteString("    " + tui.EmptyState("no nodes reported", "deploy a cluster with 'okdctl deploy'") + "\n")
	} else {
		for _, line := range nodeStatusTableLines(st.Nodes) {
			sb.WriteString("    " + line + "\n")
		}
		sb.Newline()
	}
	sb.KV("masters", strconv.Itoa(masters))
	sb.KV("workers", strconv.Itoa(workers))
	sb.KV("total", strconv.Itoa(len(st.Nodes)))
	sb.Newline()

	sb.Section("cluster operators")
	if st.DegradedOperators == 0 {
		sb.KV("degraded", "0 (all healthy)")
	} else {
		sb.KV("degraded", strconv.Itoa(st.DegradedOperators))
	}
	sb.Newline()

	if len(st.Addons) > 0 {
		sb.Section("addons")
		for _, a := range st.Addons {
			sb.KV(a.Name, a.Label())
		}
		sb.Newline()
	}

	_, err := fmt.Fprint(cmd.OutOrStdout(),
		"\n"+tui.BoxedSectionCompact(sb.String(), "cluster status", tui.DefaultBoxWidth)+"\n")
	return err
}

// nodeStatusTableLines renders the NAME/ROLE/READY node table for the status
// box through the shared tui.Table primitive: dim header, per-row red skin for
// a not-ready node. Padding is computed on plain text so a styled row's
// zero-width escapes never shift a later column.
func nodeStatusTableLines(nodes []okd.NodeStatus) []string {
	rows := make([][]string, 0, len(nodes))
	for _, n := range nodes {
		rows = append(rows, []string{n.Name, string(n.Role), yesNo(n.Ready)})
	}
	return tui.Table([]string{"NAME", "ROLE", "READY"}, rows, tui.TableOptions{
		RowStyle: func(i int) (lipgloss.Style, bool) {
			if !nodes[i].Ready {
				return tui.ErrorStyle, true
			}
			return lipgloss.Style{}, false
		},
	})
}

func runDescribeNode(cmd *cobra.Command, args []string) error {
	if err := validateFormat(describeNodeOutput); err != nil {
		return err
	}
	quietForJSON(describeNodeOutput)

	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	cl, err := clusterstatus.NewClient(projectRoot)
	if err != nil {
		return err
	}

	name := args[0]

	raw, _, ocErr := cl.GetJSON(cmd.Context(), "get", "node", name, "-o", "json")
	if ocErr != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("describe node %s", name), Err: ocErr}
	}

	n, err := clusterstatus.ParseNode([]byte(raw))
	if err != nil {
		return err
	}

	if describeNodeOutput == outputJSON {
		payload := map[string]any{
			colName: n.Name,
			"role":  n.Role,
			"ready": n.Ready,
		}
		return writeJSON(cmd.OutOrStdout(), payload)
	}

	lines := []struct{ k, v string }{
		{colName, n.Name},
		{"role", string(n.Role)},
		{"ready", yesNo(n.Ready)},
	}
	for _, ln := range lines {
		fmt.Fprintln(cmd.OutOrStdout(), tui.DottedKeyValueFull(ln.k, ln.v, tui.DefaultKeyColWidth, 0))
	}
	return nil
}

func runDescribeAddon(cmd *cobra.Command, args []string) error {
	if err := validateFormat(describeAddonOutput); err != nil {
		return err
	}
	quietForJSON(describeAddonOutput)

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	name := args[0]
	a := addon.Get(name)
	if a == nil {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("addon %q not registered; run 'okdctl addon list' to see available addons", name)}
	}

	info := a.Info()
	mgr := newAddonManager(cfg, projectRoot)
	results, _ := mgr.VerifyAll(cmd.Context())

	as := okd.AddonStatus{}
	for _, r := range results {
		if r.Name == name {
			as = okd.AddonStatus{Name: r.Name, Healthy: r.Err == nil}
			if r.Err != nil {
				as.Error = r.Err.Error()
			}
			break
		}
	}
	health := as.Label()
	if !as.Healthy && as.Error != "" {
		health += ": " + as.Error
	}

	lines := []struct{ k, jsonKey, v string }{
		{colName, colName, info.Name},
		{"display-name", "display_name", info.DisplayName},
		{"description", "description", info.Description},
		{"category", "category", info.Category},
		{"health", "health", health},
	}

	if describeAddonOutput == outputJSON {
		payload := map[string]string{}
		for _, ln := range lines {
			payload[ln.jsonKey] = ln.v
		}
		return writeJSON(cmd.OutOrStdout(), payload)
	}

	for _, ln := range lines {
		fmt.Fprintln(cmd.OutOrStdout(), tui.DottedKeyValueFull(ln.k, ln.v, tui.DefaultKeyColWidth, 0))
	}
	return nil
}
