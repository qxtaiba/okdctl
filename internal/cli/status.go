package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/version"
)

const colName = "name"

var statusFormat string

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print a post-deploy cluster summary",
	Long: `Print API reachability, node counts by role, cluster operator
health, and addon status for the deployed cluster.`,
	Example: `  okdctl status
  okdctl status --format json | jq '.nodes'
  okdctl status --format json | jq '[.nodes[] | select(.ready)] | length'`,
	RunE: runStatus,
}

var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Drill into a specific node or addon",
}

var describeNodeCmd = &cobra.Command{
	Use:     "node <name>",
	Short:   "Show detail for a cluster node",
	Example: "  okdctl describe node master-0",
	Args:    cobra.ExactArgs(1),
	RunE:    runDescribeNode,
}

var describeAddonCmd = &cobra.Command{
	Use:     "addon <name>",
	Short:   "Show detail for a registered addon",
	Example: "  okdctl describe addon flux",
	Args:    cobra.ExactArgs(1),
	RunE:    runDescribeAddon,
	ValidArgsFunction: func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return addon.Names(), cobra.ShellCompDirectiveNoFileComp
	},
}

var describeFormat string

func init() {
	statusCmd.Flags().StringVar(&statusFormat, "format", outputText, "output format: text|json")
	describeNodeCmd.Flags().StringVar(&describeFormat, "format", outputText, "output format: text|json")
	describeAddonCmd.Flags().StringVar(&describeFormat, "format", outputText, "output format: text|json")
	describeCmd.AddCommand(describeNodeCmd)
	describeCmd.AddCommand(describeAddonCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(describeCmd)
}

// statusNodeList is a minimal view of `oc get nodes -o json` for role +
// readiness parsing. Keeps the parse decoupled from corev1 schema evolution.
type statusNodeList struct {
	Items []statusNode `json:"items"`
}

type statusCondition struct {
	Type   phase.ConditionType   `json:"type"`
	Status phase.ConditionStatus `json:"status"`
}

type statusNode struct {
	Metadata struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
	Status struct {
		Conditions []statusCondition `json:"conditions"`
	} `json:"status"`
}

func (n *statusNode) isReady() bool {
	return slices.ContainsFunc(n.Status.Conditions, func(c statusCondition) bool {
		return c.Type == phase.ConditionTypeReady && c.Status == phase.ConditionStatusTrue
	})
}

func (n *statusNode) role() phase.NodeRole {
	if _, ok := n.Metadata.Labels["node-role.kubernetes.io/master"]; ok {
		return phase.RoleMaster
	}
	if _, ok := n.Metadata.Labels["node-role.kubernetes.io/worker"]; ok {
		return phase.RoleWorker
	}
	return phase.RoleUnknown
}

type clusterStatus struct {
	APIReachable      bool               `json:"api_reachable"`
	Nodes             []nodeStatusEntry  `json:"nodes"`
	DegradedOperators int                `json:"degraded_operators"`
	Addons            []addonStatusEntry `json:"addons"`
}

type nodeStatusEntry struct {
	Name  string         `json:"name"`
	Role  phase.NodeRole `json:"role"`
	Ready bool           `json:"ready"`
}

type addonStatusEntry struct {
	Name    string `json:"name"`
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

func runStatus(cmd *cobra.Command, _ []string) error {
	if err := validateFormat(statusFormat); err != nil {
		return err
	}

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	bp, err := newStatusPhase(projectRoot)
	if err != nil {
		return err
	}

	ctx := cmd.Context()

	apiOK := true
	if _, ocErr := bp.OcOutput(ctx, "get", "--raw", "/healthz"); ocErr != nil {
		apiOK = false
	}

	var nodes []nodeStatusEntry
	if raw, ocErr := bp.OcOutput(ctx, "get", "nodes", "-o", "json"); ocErr == nil {
		var nl statusNodeList
		if jsonErr := json.Unmarshal([]byte(raw), &nl); jsonErr == nil {
			for _, n := range nl.Items {
				nodes = append(nodes, nodeStatusEntry{
					Name:  n.Metadata.Name,
					Role:  n.role(),
					Ready: n.isReady(),
				})
			}
		}
	}

	degraded := 0
	if coRaw, ocErr := bp.OcOutput(ctx, "get", "clusteroperators", "--no-headers"); ocErr == nil {
		for line := range strings.Lines(strings.TrimSpace(coRaw)) {
			fields := strings.Fields(line)
			if len(fields) >= 5 && phase.ConditionStatus(fields[4]) == phase.ConditionStatusTrue {
				degraded++
			}
		}
	}

	mgr := newAddonManager(cfg, projectRoot)
	addonResults, _ := mgr.VerifyAll(ctx)
	var addonEntries []addonStatusEntry
	for _, r := range addonResults {
		e := addonStatusEntry{Name: r.Name, Healthy: r.Err == nil}
		if r.Err != nil {
			e.Error = r.Err.Error()
		}
		addonEntries = append(addonEntries, e)
	}

	st := clusterStatus{
		APIReachable:      apiOK,
		Nodes:             nodes,
		DegradedOperators: degraded,
		Addons:            addonEntries,
	}

	if statusFormat == outputJSON {
		return writeJSON(cmd.OutOrStdout(), st)
	}
	return printClusterStatus(cmd, st)
}

func printClusterStatus(cmd *cobra.Command, st clusterStatus) error {
	sb := newSummaryBuilder()
	sb.b.WriteString("\n")

	sb.section("api")
	if st.APIReachable {
		sb.kv("reachable", "yes")
	} else {
		sb.kv("reachable", "no (oc get --raw /healthz failed)")
	}
	sb.newline()

	masters := 0
	workers := 0
	for _, n := range st.Nodes {
		switch n.Role {
		case phase.RoleMaster:
			masters++
		case phase.RoleWorker:
			workers++
		}
	}
	sb.section("nodes")
	sb.kv("masters", strconv.Itoa(masters))
	sb.kv("workers", strconv.Itoa(workers))
	sb.kv("total", strconv.Itoa(len(st.Nodes)))
	sb.newline()

	sb.section("cluster operators")
	if st.DegradedOperators == 0 {
		sb.kv("degraded", "0 (all healthy)")
	} else {
		sb.kv("degraded", strconv.Itoa(st.DegradedOperators))
	}
	sb.newline()

	if len(st.Addons) > 0 {
		sb.section("addons")
		for _, a := range st.Addons {
			status := "healthy"
			if !a.Healthy {
				status = "degraded"
			}
			sb.kv(a.Name, status)
		}
		sb.newline()
	}

	_, err := fmt.Fprint(cmd.OutOrStdout(),
		"\n"+tui.BoxedSectionCompact(sb.String(), "cluster status", tui.DefaultBoxWidth)+"\n")
	return err
}

func runDescribeNode(cmd *cobra.Command, args []string) error {
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	bp, err := newStatusPhase(projectRoot)
	if err != nil {
		return err
	}

	name := args[0]
	ctx := cmd.Context()

	raw, ocErr := bp.OcOutput(ctx, "get", "node", name, "-o", "json")
	if ocErr != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("describe node %s", name), Err: ocErr}
	}

	var n statusNode
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		return fmt.Errorf("parse node json: %w", err)
	}

	if describeFormat == outputJSON {
		payload := map[string]any{
			colName: n.Metadata.Name,
			"role":  n.role(),
			"ready": n.isReady(),
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "NAME\t%s\n", n.Metadata.Name)
	fmt.Fprintf(tw, "ROLE\t%s\n", n.role())
	ready := "False"
	if n.isReady() {
		ready = "True"
	}
	fmt.Fprintf(tw, "READY\t%s\n", ready)
	return tw.Flush()
}

func runDescribeAddon(cmd *cobra.Command, args []string) error {
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

	var health string
	for _, r := range results {
		if r.Name == name {
			if r.Err == nil {
				health = "healthy"
			} else {
				health = "degraded: " + r.Err.Error()
			}
			break
		}
	}
	if health == "" {
		health = "not enabled"
	}

	lines := []struct{ k, jsonKey, v string }{
		{colName, colName, info.Name},
		{"display-name", "display_name", info.DisplayName},
		{"description", "description", info.Description},
		{"category", "category", info.Category},
		{"health", "health", health},
	}

	if describeFormat == outputJSON {
		payload := map[string]string{}
		for _, ln := range lines {
			payload[ln.jsonKey] = ln.v
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	for _, ln := range lines {
		fmt.Fprintln(cmd.OutOrStdout(), tui.DottedKeyValueFull(ln.k, ln.v, tui.DefaultKeyColWidth, 0))
	}
	return nil
}

func newStatusPhase(projectRoot string) (phase.BasePhase, error) {
	workDir := filepath.Join(projectRoot, "okd-install")
	clusterDir := phase.ClusterConfigDir(workDir)
	kcPath := filepath.Join(clusterDir, "auth", "kubeconfig")

	if !system.FileExists(kcPath) {
		return phase.BasePhase{}, &errtypes.ClusterError{
			Msg: fmt.Sprintf("kubeconfig not found at %s; run `okdctl deploy` first", kcPath),
		}
	}

	exec := executor.New(
		executor.WithEnv([]string{"KUBECONFIG=" + kcPath}),
	)
	bp := phase.NewBasePhase(version.Version, phase.WithExecutor(exec))
	return bp, nil
}
