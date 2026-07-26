package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/clusterstatus"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox/hostssh"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/sshpin"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

var (
	nodeSnapshotCreateName         string
	nodeSnapshotCreateDescription  string
	nodeSnapshotCreateSkipDrain    bool
	nodeSnapshotCreateDrainTimeout string
	nodeSnapshotCreateYes          bool
	nodeSnapshotCreateConfirm      string
	nodeSnapshotCreateDryRun       bool
	nodeSnapshotCreateAcknowledge  bool

	nodeSnapshotListOutput string

	nodeSnapshotRollbackYes         bool
	nodeSnapshotRollbackConfirm     string
	nodeSnapshotRollbackDryRun      bool
	nodeSnapshotRollbackAcknowledge bool

	nodeSnapshotDeleteYes     bool
	nodeSnapshotDeleteConfirm string
)

var nodeSnapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Manual, single-node Proxmox VM snapshots",
	Long: `Take, list, roll back, and delete point-in-time Proxmox VM snapshots for
one node at a time, over pvesh via SSH.

This is a bounded safety net, not backup/DR: snapshots are manual (no
scheduling), single-node (no fleet-wide consistency across the cluster), and
short-lived (no retention policy — you are responsible for deleting what you
create). Every snapshot is crash-consistent only: qemu-guest-agent is
disabled fleet-wide, so a snapshot captures disk state as if the VM had lost
power, not as if it had been cleanly shut down.`,
}

var nodeSnapshotCreateCmd = &cobra.Command{
	Use:   "create <node>",
	Short: "Snapshot a node's disks",
	Long: `Snapshot target's disks via pvesh. A Ready node is cordoned and drained
first unless --skip-drain is set; a NotReady node is snapshotted directly,
since a drain would only spin with nowhere to reschedule its pods.

Crash-consistent only: qemu-guest-agent is disabled fleet-wide, so this is
equivalent to the VM losing power, not a clean shutdown.

Create refuses to run while a marker from any other in-flight node op is
recorded, since snapshot is not resumable and would otherwise overwrite that
op's resume trail. --acknowledge-interrupted-op overrides the marker and
proceeds.`,
	Example: `  okdctl node snapshot create worker0 --yes --confirm-cluster grappleberry
  okdctl node snapshot create worker0 --name pre-upgrade --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runNodeSnapshotCreate,
}

var nodeSnapshotListCmd = &cobra.Command{
	Use:     "list <node>",
	Short:   "List a node's Proxmox snapshots",
	Long:    `List target's Proxmox snapshots. Read-only; no confirmation gate.`,
	Example: "  okdctl node snapshot list worker0\n  okdctl node snapshot list worker0 --output json",
	Args:    cobra.ExactArgs(1),
	RunE:    runNodeSnapshotList,
}

var nodeSnapshotRollbackCmd = &cobra.Command{
	Use:   "rollback <node> <name>",
	Short: "Roll back a node's disks to a prior snapshot",
	Long: `Restore target's disks to name and power the VM back on — pvesh passes
-start 1 unconditionally, so a VM that was deliberately powered off comes
back up too.

A master rollback is quorum-sensitive: a crash-consistent snapshot can leave
etcd's Raft term or rook-ceph's OSD state stale relative to peers that kept
running, so the op refuses to start against an already-unhealthy quorum and
re-verifies health before the node is uncordoned. Any failure from the
cordon onward leaves the node cordoned; the error names the failing stage.

Rollback refuses to run while a marker from any other in-flight node op is
recorded, since snapshot is not resumable and would otherwise overwrite that
op's resume trail. --acknowledge-interrupted-op overrides the marker and
proceeds.`,
	Example: `  okdctl node snapshot rollback worker0 pre-upgrade --yes --confirm-cluster grappleberry
  okdctl node snapshot rollback worker0 pre-upgrade --dry-run`,
	Args: cobra.ExactArgs(2),
	RunE: runNodeSnapshotRollback,
}

var nodeSnapshotDeleteCmd = &cobra.Command{
	Use:     "delete <node> <name>",
	Short:   "Delete a node's Proxmox snapshot",
	Long:    `Remove name from target. Does not touch VM power state or cordon status.`,
	Example: "  okdctl node snapshot delete worker0 pre-upgrade --yes --confirm-cluster grappleberry",
	Args:    cobra.ExactArgs(2),
	RunE:    runNodeSnapshotDelete,
}

func init() {
	nodeSnapshotCreateCmd.Flags().StringVar(&nodeSnapshotCreateName, "name", "", "snapshot name (default okdctl-<UTC timestamp>)")
	nodeSnapshotCreateCmd.Flags().StringVar(&nodeSnapshotCreateDescription, "description", "", "optional snapshot description (single token starting with a letter or digit: no spaces; use dashes or underscores)")
	nodeSnapshotCreateCmd.Flags().BoolVar(&nodeSnapshotCreateSkipDrain, "skip-drain", false, "skip cordon/drain before snapshotting a Ready node")
	nodeSnapshotCreateCmd.Flags().StringVar(&nodeSnapshotCreateDrainTimeout, "drain-timeout", "10m", "drain timeout when the node is cordoned first")
	nodeSnapshotCreateCmd.Flags().BoolVarP(&nodeSnapshotCreateYes, "yes", "y", false, "skip confirmation prompt")
	nodeSnapshotCreateCmd.Flags().StringVar(&nodeSnapshotCreateConfirm, "confirm-cluster", "", "required with --yes; must equal the config cluster name")
	nodeSnapshotCreateCmd.Flags().BoolVar(&nodeSnapshotCreateDryRun, flagDryRun, false, "report what would happen without creating a snapshot")
	nodeSnapshotCreateCmd.Flags().BoolVar(&nodeSnapshotCreateAcknowledge, "acknowledge-interrupted-op", false, "override a stranded marker left by an unrelated op and proceed fresh")

	nodeSnapshotListCmd.Flags().StringVarP(&nodeSnapshotListOutput, flagOutput, flagOutputShort, outputText, "output format: text|json")
	registerOutputCompletion(nodeSnapshotListCmd)

	nodeSnapshotRollbackCmd.Flags().BoolVarP(&nodeSnapshotRollbackYes, "yes", "y", false, "skip confirmation prompt")
	nodeSnapshotRollbackCmd.Flags().StringVar(&nodeSnapshotRollbackConfirm, "confirm-cluster", "", "required with --yes; must equal the config cluster name")
	nodeSnapshotRollbackCmd.Flags().BoolVar(&nodeSnapshotRollbackDryRun, flagDryRun, false, "report what would happen without rolling back")
	nodeSnapshotRollbackCmd.Flags().BoolVar(&nodeSnapshotRollbackAcknowledge, "acknowledge-interrupted-op", false, "override a stranded marker left by an unrelated op and proceed fresh")

	nodeSnapshotDeleteCmd.Flags().BoolVarP(&nodeSnapshotDeleteYes, "yes", "y", false, "skip confirmation prompt")
	nodeSnapshotDeleteCmd.Flags().StringVar(&nodeSnapshotDeleteConfirm, "confirm-cluster", "", "required with --yes; must equal the config cluster name")

	nodeSnapshotCmd.AddCommand(nodeSnapshotCreateCmd)
	nodeSnapshotCmd.AddCommand(nodeSnapshotListCmd)
	nodeSnapshotCmd.AddCommand(nodeSnapshotRollbackCmd)
	nodeSnapshotCmd.AddCommand(nodeSnapshotDeleteCmd)
	nodeCmd.AddCommand(nodeSnapshotCmd)
}

// buildSnapshotRunner wires a node.Runner for the pvesh-over-SSH snapshot
// surface: it verifies the Proxmox host's SSH fingerprint, builds the
// RemoteISOParams the runner's HostsshSnapshotClient needs, and acquires the
// project run lock. Unlike buildNodeRunner it never touches Terraform
// (snapshots don't need the terraform root) and never wires Power
// (proxmox.NewPowerCycler is REST/API-credential
// based; snapshots are SSH-key based), keeping the pvesh surface separate
// from cluster stop/start's REST surface.
func buildSnapshotRunner(ctx context.Context, cfg *config.Config, dryRun bool) (*nodeRunnerCtx, error) {
	if cfg.Provider.Proxmox == nil {
		return nil, &errtypes.ConfigError{Msg: "node snapshot requires provider.proxmox to be configured"}
	}
	px := cfg.Provider.Proxmox

	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return nil, err
	}

	log := tui.SimpleLogger()
	host := hostssh.ProxmoxBareHost(px.Host)
	knownHostsPath, err := sshpin.Verify(ctx, host, px.SSHHostFingerprint, px.RequirePinnedFingerprint, log)
	if err != nil {
		return nil, err
	}

	cl, err := clusterstatus.NewClient(projectRoot)
	if err != nil {
		return nil, err
	}

	lock, err := runlock.Acquire(projectRoot, "node snapshot")
	if err != nil {
		return nil, err
	}

	tfEnv := cfg.TerraformEnvName()
	runner := &node.Runner{
		Cluster:     cl,
		Cfg:         cfg,
		ConfigPath:  cfgFile,
		ProjectRoot: projectRoot,
		WorkDir:     workspace.WorkDir(projectRoot),
		EnvDir:      workspace.TerraformEnvDir(projectRoot, tfEnv),
		RunID:       tui.RunID(),
		DryRun:      dryRun,
		Log:         log,
		Reporter:    func(desc string) func() { return tui.StartSpinner(ctx, desc) },
		Proxmox: &hostssh.RemoteISOParams{
			Host:           host,
			Node:           px.Node,
			Exec:           executor.New(executor.WithWorkDir(projectRoot)),
			Log:            log,
			KnownHostsPath: knownHostsPath,
		},
		Snapshot:            node.HostsshSnapshotClient{},
		NodeReadyTimeout:    node.DefaultNodeReadyTimeout,
		EtcdGateTimeout:     node.DefaultEtcdGateTimeout,
		CephGateTimeout:     node.DefaultCephGateTimeout,
		SnapshotTaskTimeout: node.DefaultSnapshotTaskTimeout,
	}

	return &nodeRunnerCtx{
		runner:  runner,
		dryRun:  dryRun,
		release: lock.Release,
	}, nil
}

// nodeSnapshotGate runs the write-op consent flow shared by create,
// rollback, and delete: the --yes/--confirm-cluster pairing check always
// runs; the interactive gate (single y/N, or rollback's two-stage typed-
// cluster-name for its genuinely destructive VM power-on + disk restore) is
// skipped under --yes or --dry-run, so a dry-run never blocks on a prompt
// whose only purpose is deciding whether to mutate anything — mirroring
// buildNodeRunner's Preview/Confirm split. warnMsg fires immediately before
// the interactive prompt only: a --yes run sees the crash-consistency
// notice solely from the runner's own log line, not a duplicate here.
func nodeSnapshotGate(ctx context.Context, verb string, twoStage, yes, dryRun bool, confirmCluster, clusterName, warnMsg string, warnFields ...tui.LogField) (bool, error) {
	if err := confirmClusterMatches(yes, confirmCluster, clusterName, verb); err != nil {
		return false, err
	}
	if yes || dryRun {
		return true, nil
	}
	if warnMsg != "" {
		tui.Warn(warnMsg, warnFields...)
	}
	return runNodeGate(ctx, twoStage, clusterName)
}

func runNodeSnapshotCreate(cmd *cobra.Command, args []string) error {
	// Validated here as well as in hostssh so a --dry-run previews only
	// values a real run would accept, instead of echoing a name/description
	// the create itself would reject.
	if nodeSnapshotCreateName != "" {
		if err := hostssh.ValidateSnapshotName(nodeSnapshotCreateName); err != nil {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("invalid --name %q", nodeSnapshotCreateName), Err: err}
		}
	}
	if err := hostssh.ValidateSnapshotDescription(nodeSnapshotCreateDescription); err != nil {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("invalid --description %q", nodeSnapshotCreateDescription), Err: err}
	}

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}
	target := args[0]

	proceed, err := nodeSnapshotGate(cmd.Context(), "node snapshot create", false,
		nodeSnapshotCreateYes, nodeSnapshotCreateDryRun, nodeSnapshotCreateConfirm, cfg.Cluster.Name,
		"snapshot is crash-consistent only (qemu-guest-agent is disabled fleet-wide); not a substitute for backup/DR",
		tui.LF("node", target))
	if err != nil {
		return err
	}
	if !proceed {
		tui.Info("cancelled")
		return nil
	}

	rc, err := buildSnapshotRunner(cmd.Context(), cfg, nodeSnapshotCreateDryRun)
	if err != nil {
		return err
	}
	defer rc.cleanup()

	name, err := rc.runner.CreateSnapshot(cmd.Context(), target, node.SnapshotCreateOptions{
		Name:         nodeSnapshotCreateName,
		Description:  nodeSnapshotCreateDescription,
		SkipDrain:    nodeSnapshotCreateSkipDrain,
		DrainTimeout: nodeSnapshotCreateDrainTimeout,
		Acknowledge:  nodeSnapshotCreateAcknowledge,
	})
	if err != nil {
		return err
	}
	if !nodeSnapshotCreateDryRun {
		fmt.Fprintln(cmd.OutOrStdout(), name)
	}
	return nil
}

// runNodeSnapshotMutate is the shared shape behind rollback and delete: gate
// consent, build the snapshot runner, and invoke op — the cluster.go
// runClusterPower pattern applied to the pvesh surface. create keeps its own
// RunE since it must thread the (possibly auto-generated) snapshot name back
// to stdout on success.
func runNodeSnapshotMutate(cmd *cobra.Command, verb string, twoStage, yes, dryRun bool, confirmCluster, warnMsg, target, snapname string, op func(rc *nodeRunnerCtx) error) error {
	// Validated here as well as in hostssh so a --dry-run previews only names
	// a real run would accept.
	if err := hostssh.ValidateSnapshotName(snapname); err != nil {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("invalid snapshot name %q", snapname), Err: err}
	}

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	proceed, err := nodeSnapshotGate(cmd.Context(), verb, twoStage, yes, dryRun, confirmCluster, cfg.Cluster.Name,
		warnMsg, tui.LF("node", target), tui.LF("snapshot", snapname))
	if err != nil {
		return err
	}
	if !proceed {
		tui.Info("cancelled")
		return nil
	}

	rc, err := buildSnapshotRunner(cmd.Context(), cfg, dryRun)
	if err != nil {
		return err
	}
	defer rc.cleanup()

	return op(rc)
}

func runNodeSnapshotRollback(cmd *cobra.Command, args []string) error {
	target, snapname := args[0], args[1]
	return runNodeSnapshotMutate(cmd, "node snapshot rollback", true,
		nodeSnapshotRollbackYes, nodeSnapshotRollbackDryRun, nodeSnapshotRollbackConfirm,
		"rollback restores disk state from a crash-consistent snapshot and powers the vm back on regardless of its current state; a master rollback is quorum-sensitive",
		target, snapname,
		func(rc *nodeRunnerCtx) error {
			return rc.runner.RollbackSnapshot(cmd.Context(), target, snapname, node.SnapshotRollbackOptions{Acknowledge: nodeSnapshotRollbackAcknowledge})
		})
}

func runNodeSnapshotDelete(cmd *cobra.Command, args []string) error {
	target, snapname := args[0], args[1]
	return runNodeSnapshotMutate(cmd, "node snapshot delete", false,
		nodeSnapshotDeleteYes, false, nodeSnapshotDeleteConfirm,
		"deleting a snapshot is irreversible",
		target, snapname,
		func(rc *nodeRunnerCtx) error { return rc.runner.DeleteSnapshot(cmd.Context(), target, snapname) })
}

func runNodeSnapshotList(cmd *cobra.Command, args []string) error {
	if err := validateFormat(nodeSnapshotListOutput); err != nil {
		return err
	}
	quietForJSON(nodeSnapshotListOutput)

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	rc, err := buildSnapshotRunner(cmd.Context(), cfg, false)
	if err != nil {
		return err
	}
	defer rc.cleanup()

	snapshots, err := rc.runner.ListSnapshots(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	entries := toNodeSnapshotEntries(snapshots)

	if nodeSnapshotListOutput == outputJSON {
		return writeJSON(cmd.OutOrStdout(), entries)
	}
	return printNodeSnapshotList(cmd.OutOrStdout(), entries)
}

// nodeSnapshotEntry is one row of `okdctl node snapshot list --output
// json`; see docs/cli/json-schema.md for the documented, stable shape.
type nodeSnapshotEntry struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	SnapTime    string `json:"snap_time,omitempty"`
	Parent      string `json:"parent,omitempty"`
}

func toNodeSnapshotEntries(snapshots []hostssh.SnapshotInfo) []nodeSnapshotEntry {
	entries := make([]nodeSnapshotEntry, 0, len(snapshots))
	for _, s := range snapshots {
		e := nodeSnapshotEntry{Name: s.Name, Description: s.Description, Parent: s.Parent}
		if s.SnapTime > 0 {
			e.SnapTime = time.Unix(s.SnapTime, 0).UTC().Format(time.RFC3339)
		}
		entries = append(entries, e)
	}
	return entries
}

func printNodeSnapshotList(w io.Writer, entries []nodeSnapshotEntry) error {
	if len(entries) == 0 {
		_, err := fmt.Fprintln(w, "no snapshots found")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSNAPTIME\tPARENT\tDESCRIPTION")
	for _, e := range entries {
		snapTime := e.SnapTime
		if snapTime == "" {
			snapTime = "-"
		}
		parent := e.Parent
		if parent == "" {
			parent = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", stripControl(e.Name), snapTime, stripControl(parent), stripControl(e.Description))
	}
	return tw.Flush()
}

// stripControl drops control characters from a value before it reaches the
// operator's terminal. okdctl-created snapshot fields are charset-safe, but a
// snapshot created in the Proxmox UI carries free-text — a hostile description
// could otherwise inject terminal escapes or fabricate extra listing rows.
// JSON output needs no equivalent: encoding/json escapes control characters.
func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
