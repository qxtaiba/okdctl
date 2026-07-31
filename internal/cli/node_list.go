package cli

import (
	"fmt"
	"io"
	"strconv"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/clusterstatus"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/provision"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// Sizing-drift values for nodeListEntry.Drift. "pending" means config and
// terraform.tfvars disagree on the node's role sizing — a resize was staged
// (or the config was hand-edited) but not yet fully applied; "unknown" means
// terraform.tfvars has not been rendered yet, so there is nothing to compare.
const (
	driftNone    = "none"
	driftPending = "pending"
	driftUnknown = "unknown"
)

var nodeListOutput string

var nodeListCmd = &cobra.Command{
	Use:   cmdNameList,
	Short: "List cluster nodes with role, readiness, and sizing drift",
	Long: `List every cluster node with its role, readiness, terraform count
index, sizing-drift indicator, and any in-flight node op.

The drift indicator compares the config file's per-role cpu/memory to what
was last rendered into terraform.tfvars — it is not a live VM query (okdctl
fetches no per-guest Proxmox sizing anywhere today), so "pending" means a
sizing change is staged in the workspace, not that a specific node's guest
has actually been resized yet. "unknown" means terraform.tfvars has not been
rendered at all.`,
	Example: "  okdctl node list\n  okdctl node list --output json",
	RunE:    runNodeList,
}

func init() {
	nodeListCmd.Flags().StringVarP(&nodeListOutput, flagOutput, flagOutputShort, outputText, "output format: text|json")
	registerOutputCompletion(nodeListCmd)
	nodeCmd.AddCommand(nodeListCmd)
}

// nodeListEntry is one row of `okdctl node list --output json`; see
// docs/cli/json-schema.md for the documented, stable shape.
type nodeListEntry struct {
	Name        string             `json:"name"`
	Role        nodetypes.NodeRole `json:"role"`
	Ready       bool               `json:"ready"`
	TFIndex     *int               `json:"tf_index,omitempty"`
	Drift       string             `json:"drift"`
	DriftDetail string             `json:"drift_detail,omitempty"`
	InFlightOp  string             `json:"in_flight_op,omitempty"`
}

func runNodeList(cmd *cobra.Command, _ []string) error {
	if err := validateFormat(nodeListOutput); err != nil {
		return err
	}
	quietForJSON(nodeListOutput)

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	cl, err := clusterstatus.NewClient(projectRoot)
	if err != nil {
		return err
	}

	nodes, err := cl.ListNodes(cmd.Context())
	if err != nil {
		return err
	}

	side := loadNodeListSideData(cfg, projectRoot)
	entries := buildNodeListEntries(nodes, cfg, side)
	unattached := unattachedOpNote(side.marker, nodes)

	if nodeListOutput == outputJSON {
		return writeJSON(cmd.OutOrStdout(), entries)
	}
	return printNodeList(cmd.OutOrStdout(), entries, unattached)
}

// nodeListSideData is the non-cluster context `okdctl node list` folds onto
// each row: the sizing recorded in terraform.tfvars (for drift) and the
// in-flight op marker, if any. Both are read best-effort — a read failure
// degrades the affected column to "unknown"/absent rather than failing the
// whole listing, matching clusterstatus.Collect's degrade-gracefully policy.
type nodeListSideData struct {
	tfSizing      provision.TerraformVarsSizing
	tfSizingFound bool
	marker        *node.OpMarker
}

func loadNodeListSideData(cfg *config.Config, projectRoot string) nodeListSideData {
	envDir := workspace.TerraformEnvDir(projectRoot, cfg.TerraformEnvName())
	sizing, found, err := provision.ReadTerraformVarsSizing(envDir)
	if err != nil {
		logutil.Warn("node list: read terraform.tfvars sizing failed; drift will show as unknown", logutil.LF("err", err))
		found = false
	}

	workDir := workspace.WorkDir(projectRoot)
	marker, err := node.ReadOpMarker(workDir, cfg.Cluster.Name)
	if err != nil {
		logutil.Warn("node list: read in-flight op marker failed", logutil.LF("err", err))
		marker = nil
	}

	return nodeListSideData{tfSizing: sizing, tfSizingFound: found, marker: marker}
}

func buildNodeListEntries(nodes []cluster.NodeDetail, cfg *config.Config, side nodeListSideData) []nodeListEntry {
	entries := make([]nodeListEntry, 0, len(nodes))
	for _, n := range nodes {
		e := nodeListEntry{Name: n.Name, Role: n.Role, Ready: n.Ready}
		if idx, ok := cluster.NodeIndex(n.Name); ok {
			e.TFIndex = &idx
		}
		e.Drift, e.DriftDetail = roleSizingDrift(cfg, n.Role, side.tfSizing, side.tfSizingFound)
		if side.marker != nil && side.marker.Target == n.Name {
			e.InFlightOp = fmt.Sprintf("%s (%s)", side.marker.Op, side.marker.Step)
		}
		entries = append(entries, e)
	}
	return entries
}

// unattachedOpNote reports a marker whose Target matches no listed node —
// see the unattached-op text note. Empty when there is no marker or it is
// already attached to a listed node via in_flight_op.
func unattachedOpNote(marker *node.OpMarker, nodes []cluster.NodeDetail) string {
	if marker == nil {
		return ""
	}
	for _, n := range nodes {
		if n.Name == marker.Target {
			return ""
		}
	}
	return fmt.Sprintf("%s (%s) on %s", marker.Op, marker.Step, marker.Target)
}

// roleSizingDrift compares cfg's role sizing to sizing (parsed from
// terraform.tfvars). found=false means terraform.tfvars has not been
// rendered, so drift cannot be assessed.
func roleSizingDrift(cfg *config.Config, role nodetypes.NodeRole, sizing provision.TerraformVarsSizing, found bool) (status, detail string) {
	if !found {
		return driftUnknown, ""
	}
	var cfgCPU, cfgMem, tfCPU, tfMem int
	switch role {
	case nodetypes.RoleMaster:
		cfgCPU, cfgMem = cfg.Topology.ControlPlane.CPU, cfg.Topology.ControlPlane.MemoryMB
		tfCPU, tfMem = sizing.MasterCPU, sizing.MasterMemoryMB
	case nodetypes.RoleWorker:
		cfgCPU, cfgMem = cfg.Topology.Workers.CPU, cfg.Topology.Workers.MemoryMB
		tfCPU, tfMem = sizing.WorkerCPU, sizing.WorkerMemoryMB
	default:
		return driftUnknown, ""
	}
	if cfgCPU == tfCPU && cfgMem == tfMem {
		return driftNone, ""
	}
	return driftPending, fmt.Sprintf("config %dMiB/%dcpu vs tfvars %dMiB/%dcpu", cfgMem, cfgCPU, tfMem, tfCPU)
}

// printNodeList renders the text table through the shared tui.Table look
// (dim header, red not-ready rows), plus a trailing note when unattachedOp
// is non-empty (the unattached-op text note). This surface prints outside a
// box, so each line goes through tui.Downsample to honor pipes and NO_COLOR.
func printNodeList(w io.Writer, entries []nodeListEntry, unattachedOp string) error {
	if len(entries) == 0 {
		if _, err := fmt.Fprintln(w, "no nodes found"); err != nil {
			return err
		}
	} else {
		rows := make([][]string, 0, len(entries))
		for _, e := range entries {
			idx := "-"
			if e.TFIndex != nil {
				idx = strconv.Itoa(*e.TFIndex)
			}
			op := e.InFlightOp
			if op == "" {
				op = "-"
			}
			rows = append(rows, []string{e.Name, e.Role.String(), yesNo(e.Ready), idx, e.Drift, op})
		}
		lines := tui.Table([]string{"NAME", "ROLE", "READY", "TF-INDEX", "DRIFT", "OP"}, rows, tui.TableOptions{
			RowStyle: func(i int) (lipgloss.Style, bool) {
				if !entries[i].Ready {
					return tui.ErrorStyle, true
				}
				return lipgloss.Style{}, false
			},
		})
		for _, line := range lines {
			if _, err := fmt.Fprintln(w, tui.Downsample(line)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w, "\nDRIFT compares config sizing to terraform.tfvars on disk, not live VM state."); err != nil {
			return err
		}
	}
	if unattachedOp == "" {
		return nil
	}
	_, err := fmt.Fprintf(w, "\nin-flight op: %s — not attached to a listed node\n", unattachedOp)
	return err
}
