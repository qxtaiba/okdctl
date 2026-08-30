package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/provision"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui"
)

func intPtr(i int) *int { return &i }

// headerColumns locates each header's byte offset and returns a slicer
// extracting the same column from any row.
func headerColumns(t *testing.T, header string, headers []string) (colStarts []int, col func(line string, i int) string) {
	t.Helper()
	colStarts = make([]int, len(headers))
	for i, h := range headers {
		idx := strings.Index(header, h)
		if idx == -1 {
			t.Fatalf("header missing %q: %q", h, header)
		}
		colStarts[i] = idx
	}
	col = func(line string, i int) string {
		start := min(colStarts[i], len(line))
		end := len(line)
		if i+1 < len(colStarts) {
			end = min(colStarts[i+1], len(line))
		}
		return strings.TrimSpace(line[start:end])
	}
	return colStarts, col
}

func TestRoleSizingDrift(t *testing.T) {
	cfg := &config.Config{
		Topology: config.TopologyConfig{
			ControlPlane: config.NodeConfig{CPU: 4, MemoryMB: 8192, DiskGB: 50},
			Workers:      config.NodeConfig{CPU: 8, MemoryMB: 16384, DiskGB: 50},
		},
	}
	inSync := provision.TerraformVarsSizing{MasterCPU: 4, MasterMemoryMB: 8192, MasterOSDiskGB: 50, WorkerCPU: 8, WorkerMemoryMB: 16384, WorkerOSDiskGB: 50}
	stale := provision.TerraformVarsSizing{MasterCPU: 4, MasterMemoryMB: 8192, MasterOSDiskGB: 50, WorkerCPU: 8, WorkerMemoryMB: 8192, WorkerOSDiskGB: 50}

	cases := []struct {
		name       string
		role       nodetypes.NodeRole
		cfgMod     func(*config.Config)
		sizing     provision.TerraformVarsSizing
		found      bool
		wantStatus string
		wantDetail bool
	}{
		{name: "not rendered yet", role: nodetypes.RoleMaster, sizing: provision.TerraformVarsSizing{}, found: false, wantStatus: driftUnknown, wantDetail: false},
		{name: "master in sync", role: nodetypes.RoleMaster, sizing: inSync, found: true, wantStatus: driftNone, wantDetail: false},
		{name: "worker in sync", role: nodetypes.RoleWorker, sizing: inSync, found: true, wantStatus: driftNone, wantDetail: false},
		{name: "worker drifted", role: nodetypes.RoleWorker, sizing: stale, found: true, wantStatus: driftPending, wantDetail: true},
		{name: "unknown role", role: nodetypes.RoleUnknown, sizing: inSync, found: true, wantStatus: driftUnknown, wantDetail: false},
		{
			name:       "disk drift only",
			role:       nodetypes.RoleMaster,
			cfgMod:     func(c *config.Config) { c.Topology.ControlPlane.DiskGB = 100 },
			sizing:     provision.TerraformVarsSizing{MasterCPU: 4, MasterMemoryMB: 8192, MasterOSDiskGB: 50},
			found:      true,
			wantStatus: driftPending,
			wantDetail: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runCfg := cfg
			if tc.cfgMod != nil {
				clone := *cfg
				tc.cfgMod(&clone)
				runCfg = &clone
			}
			status, detail := roleSizingDrift(runCfg, tc.role, tc.sizing, tc.found)
			if status != tc.wantStatus {
				t.Errorf("status = %q, want %q", status, tc.wantStatus)
			}
			if tc.wantDetail && detail == "" {
				t.Error("want non-empty detail, got empty")
			}
			if !tc.wantDetail && detail != "" {
				t.Errorf("want empty detail, got %q", detail)
			}
			if tc.name == "disk drift only" {
				if !strings.Contains(detail, "50GiB") || !strings.Contains(detail, "100GiB") {
					t.Errorf("detail %q must mention both disk values (50GiB and 100GiB)", detail)
				}
			}
		})
	}
}

func TestBuildNodeListEntries(t *testing.T) {
	cfg := &config.Config{
		Topology: config.TopologyConfig{
			ControlPlane: config.NodeConfig{CPU: 4, MemoryMB: 8192},
			Workers:      config.NodeConfig{CPU: 4, MemoryMB: 8192},
		},
	}
	nodes := []cluster.NodeDetail{
		{Name: "master-0", Role: nodetypes.RoleMaster, Ready: true},
		{Name: "worker-2", Role: nodetypes.RoleWorker, Ready: false},
		{Name: "foreign-node", Role: nodetypes.RoleUnknown, Ready: true},
	}
	side := nodeListSideData{
		tfSizing:      provision.TerraformVarsSizing{MasterCPU: 4, MasterMemoryMB: 8192, WorkerCPU: 4, WorkerMemoryMB: 8192},
		tfSizingFound: true,
		marker:        &node.OpMarker{Op: node.OpResize, Target: "worker-2", Step: node.StepTFApply},
	}

	got := buildNodeListEntries(nodes, cfg, side)
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}

	if got[0].TFIndex == nil || *got[0].TFIndex != 0 {
		t.Errorf("master-0 TFIndex = %v, want 0", got[0].TFIndex)
	}
	if got[0].Drift != driftNone {
		t.Errorf("master-0 Drift = %q, want %q", got[0].Drift, driftNone)
	}
	if got[0].InFlightOp != "" {
		t.Errorf("master-0 InFlightOp = %q, want empty (marker targets worker-2)", got[0].InFlightOp)
	}

	if got[1].TFIndex == nil || *got[1].TFIndex != 2 {
		t.Errorf("worker-2 TFIndex = %v, want 2", got[1].TFIndex)
	}
	if got[1].InFlightOp != "resize (tf-apply)" {
		t.Errorf("worker-2 InFlightOp = %q, want %q", got[1].InFlightOp, "resize (tf-apply)")
	}

	if got[2].TFIndex != nil {
		t.Errorf("foreign-node TFIndex = %v, want nil (no trailing digits)", got[2].TFIndex)
	}
	if got[2].Drift != driftUnknown {
		t.Errorf("foreign-node Drift = %q, want %q (unknown role)", got[2].Drift, driftUnknown)
	}
}

func TestNodeListEntryJSONShape(t *testing.T) {
	withIndex := nodeListEntry{Name: "master-0", Role: nodetypes.RoleMaster, Ready: true, TFIndex: intPtr(0), Drift: driftNone}
	data, err := json.Marshal(withIndex)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	for _, want := range []string{`"name":"master-0"`, `"role":"master"`, `"ready":true`, `"tf_index":0`, `"drift":"none"`} {
		if !strings.Contains(s, want) {
			t.Errorf("json output %q missing %q", s, want)
		}
	}
	for _, absent := range []string{"drift_detail", "in_flight_op"} {
		if strings.Contains(s, absent) {
			t.Errorf("json output %q must omit empty %q", s, absent)
		}
	}

	noIndex := nodeListEntry{Name: "foreign", Role: nodetypes.RoleUnknown, Drift: driftUnknown}
	data, err = json.Marshal(noIndex)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "tf_index") {
		t.Errorf("json output %q must omit nil tf_index", string(data))
	}
}

// profile is pinned to no-color so byte-offset column math stays valid on ANSI-styled rows.
func TestPrintNodeListAlignsColumnsWithLongNames(t *testing.T) {
	tui.SetColorProfileFor(&bytes.Buffer{})
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })

	entries := []nodeListEntry{
		{Name: "m0", Role: nodetypes.RoleMaster, Ready: true, TFIndex: intPtr(0), Drift: driftNone},
		{
			Name: "worker-extraordinarily-long-hostname-12", Role: nodetypes.RoleWorker, Ready: false,
			TFIndex: intPtr(12), Drift: driftPending, DriftDetail: "config 8192MiB/4cpu/50GiB vs tfvars 4096MiB/4cpu/50GiB",
			InFlightOp: "resize (tf-apply)",
		},
	}
	var buf bytes.Buffer
	if err := printNodeList(&buf, entries, ""); err != nil {
		t.Fatalf("printNodeList: %v", err)
	}

	lines := strings.Split(buf.String(), "\n")
	if len(lines) < 3 {
		t.Fatalf("want header + 2 data rows + footer, got %d lines:\n%s", len(lines), buf.String())
	}
	header, row0, row1 := lines[0], lines[1], lines[2]

	// offsets come from the header text; a substring search would
	// false-positive on "worker" inside the long node name
	colStarts, col := headerColumns(t, header, []string{"NAME", "ROLE", "READY", "TF-INDEX", "DRIFT", "OP"})

	const roleCol, driftCol, opCol = 1, 4, 5
	if got := col(row0, roleCol); got != nodetypes.RoleMaster.String() {
		t.Errorf("row0 ROLE column = %q, want %q (header offset %d):\n%s", got, nodetypes.RoleMaster.String(), colStarts[roleCol], buf.String())
	}
	if got := col(row1, roleCol); got != nodetypes.RoleWorker.String() {
		t.Errorf("row1 ROLE column = %q, want %q (header offset %d):\n%s", got, nodetypes.RoleWorker.String(), colStarts[roleCol], buf.String())
	}
	if got := col(row0, driftCol); got != driftNone {
		t.Errorf("row0 DRIFT column = %q, want %q", got, driftNone)
	}
	if got := col(row1, driftCol); got != driftPending {
		t.Errorf("row1 DRIFT column = %q, want %q", got, driftPending)
	}
	if got := col(row1, opCol); got != "resize (tf-apply)" {
		t.Errorf("row1 OP column = %q, want %q", got, "resize (tf-apply)")
	}
	if strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("node list output must carry no ANSI under a no-color profile:\n%q", buf.String())
	}
}

func TestPrintNodeListEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := printNodeList(&buf, nil, ""); err != nil {
		t.Fatalf("printNodeList: %v", err)
	}
	if got := buf.String(); got != "no nodes found\n" {
		t.Errorf("printNodeList(nil, \"\") = %q, want %q", got, "no nodes found\n")
	}
}

func TestPrintNodeListShowsUnattachedOpNote(t *testing.T) {
	var buf bytes.Buffer
	if err := printNodeList(&buf, nil, "stop (shutdown) on grappleberry"); err != nil {
		t.Fatalf("printNodeList: %v", err)
	}
	if !strings.Contains(buf.String(), "no nodes found") || !strings.Contains(buf.String(), "in-flight op:") {
		t.Errorf("unattached-op note must surface even with an empty node table: %q", buf.String())
	}
}

func TestUnattachedOpNote(t *testing.T) {
	nodes := []cluster.NodeDetail{
		{Name: "master-0", Role: nodetypes.RoleMaster},
		{Name: "worker-2", Role: nodetypes.RoleWorker},
	}

	if got := unattachedOpNote(nil, nodes); got != "" {
		t.Errorf("no marker: got %q, want empty", got)
	}

	attached := &node.OpMarker{Op: node.OpResize, Target: "worker-2", Step: node.StepTFApply}
	if got := unattachedOpNote(attached, nodes); got != "" {
		t.Errorf("marker attached to a listed node: got %q, want empty (already surfaced via in_flight_op)", got)
	}

	clusterStop := &node.OpMarker{Op: node.OpStop, Target: "grappleberry", Step: node.StepShutdown}
	if got := unattachedOpNote(clusterStop, nodes); got != "stop (shutdown) on grappleberry" {
		t.Errorf("cluster-stop marker: got %q, want %q", got, "stop (shutdown) on grappleberry")
	}

	removedNode := &node.OpMarker{Op: node.OpRemove, Target: "worker-9", Step: node.StepDrain}
	if got := unattachedOpNote(removedNode, nodes); got != "remove (drain) on worker-9" {
		t.Errorf("marker for a removed node: got %q, want %q", got, "remove (drain) on worker-9")
	}
}

func TestLoadNodeListSideDataReadsTfvarsAndMarker(t *testing.T) {
	projectRoot := t.TempDir()
	cfg := &config.Config{
		Cluster: config.ClusterConfig{Name: "testcluster"},
		Provider: config.ProviderConfig{
			Proxmox: &config.ProxmoxConfig{ISOStorage: "iso"},
		},
		Topology: config.TopologyConfig{
			ControlPlane: config.NodeConfig{CPU: 4, MemoryMB: 8192},
			Workers:      config.NodeConfig{CPU: 4, MemoryMB: 8192},
		},
	}

	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := provision.WriteTerraformVars(cfg, envDir); err != nil {
		t.Fatalf("WriteTerraformVars: %v", err)
	}

	workDir := filepath.Join(projectRoot, "okd-install")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	markerJSON := `{"schema_version":"v1","op":"resize","target":"worker-0","step":"tf-apply",` +
		`"run_id":"run-1","cluster_name":"testcluster","timestamp":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(workDir, node.OpMarkerFileName), []byte(markerJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	side := loadNodeListSideData(cfg, projectRoot)
	if !side.tfSizingFound {
		t.Fatal("tfSizingFound = false, want true")
	}
	if side.tfSizing.MasterCPU != 4 || side.tfSizing.MasterMemoryMB != 8192 {
		t.Errorf("tfSizing = %+v", side.tfSizing)
	}
	if side.marker == nil || side.marker.Target != "worker-0" || side.marker.Op != node.OpResize {
		t.Fatalf("marker = %+v, want resize targeting worker-0", side.marker)
	}
}
