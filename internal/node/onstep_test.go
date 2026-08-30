package node

import (
	"context"
	"slices"
	"testing"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func TestOnStepObservesWorkerResizeSequence(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}},
		schedulable: true,
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	cfg := config.DefaultConfig()
	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = &fakePower{}

	var got []string
	r.OnStep = func(target string, step Step) { got = append(got, target+":"+string(step)) }

	if err := r.Resize(context.Background(), ResizeScope{Node: "worker0"}, ResizeOptions{MemoryMB: 16384}); err != nil {
		t.Fatalf("resize: %v", err)
	}
	want := []string{
		"worker0:cordon", "worker0:drain", "worker0:tf-apply",
		"worker0:power-cycle", "worker0:uncordon",
	}
	if !slices.Equal(got, want) {
		t.Errorf("OnStep sequence = %v, want %v", got, want)
	}
}
