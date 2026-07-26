package lifecycle

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func TestMatchRowMapsStepAndReporterEvents(t *testing.T) {
	rows := GateRows(node.OpResize, nodetypes.RoleMaster, false)
	cases := []struct {
		ev   *ExecEvent
		want string
	}{
		{&ExecEvent{Step: node.StepCordon}, "cordon + drain"},
		{&ExecEvent{Step: node.StepDrain}, "cordon + drain"},
		{&ExecEvent{Step: node.StepTFApply}, "terraform apply (in-place update)"},
		{&ExecEvent{Step: node.StepPowerCycle}, "power-cycle vm"},
		{&ExecEvent{Step: node.StepUncordon}, "uncordon + ceph health gate"},
		{&ExecEvent{Desc: "waiting for etcd health (pre-master0)"}, "etcd health gate (pre)"},
		{&ExecEvent{Desc: "waiting for etcd health (post-master0)"}, "etcd health gate (post)"},
		{&ExecEvent{Desc: "waiting for node master0 to become ready"}, "wait for node ready"},
		{&ExecEvent{Desc: "waiting for ceph health (post-master0)"}, "uncordon + ceph health gate"},
		{&ExecEvent{Desc: "cordoning and draining master0"}, "cordon + drain"},
		{&ExecEvent{Desc: "applying terraform change to m.master[0]"}, "terraform apply (in-place update)"},
		{&ExecEvent{Desc: "power-cycling vm to realize the new sizing"}, "power-cycle vm"},
	}
	for _, tc := range cases {
		idx := matchRow(rows, tc.ev)
		if idx < 0 || rows[idx] != tc.want {
			t.Errorf("matchRow(%+v) = %d, want row %q", tc.ev, idx, tc.want)
		}
	}
	if matchRow(rows, &ExecEvent{Desc: "something new"}) != -1 {
		t.Error("unknown desc must not match")
	}
}

func TestMatchRowRemoveAndAdd(t *testing.T) {
	removeRows := GateRows(node.OpRemove, nodetypes.RoleWorker, false)
	if idx := matchRow(removeRows, &ExecEvent{Step: node.StepDeleteK8s}); idx < 0 || removeRows[idx] != "delete kubernetes node" {
		t.Errorf("delete-node step must match its row, got %d", idx)
	}
	if idx := matchRow(removeRows, &ExecEvent{Desc: "waiting for ceph health (post-remove-worker2)"}); idx < 0 || removeRows[idx] != "ceph health gate" {
		t.Errorf("ceph desc must match the standalone ceph row, got %d", idx)
	}

	addRows := GateRows(node.OpAdd, nodetypes.RoleWorker, false)
	for step, want := range map[node.Step]string{
		node.StepBuildISO:  "build iso",
		node.StepUploadISO: "upload iso",
		node.StepTFApply:   "terraform apply (create)",
		node.StepWaitJoin:  "wait for join + ready",
	} {
		if idx := matchRow(addRows, &ExecEvent{Step: step}); idx < 0 || addRows[idx] != want {
			t.Errorf("step %s: matchRow = %d, want row %q", step, idx, want)
		}
	}
}
