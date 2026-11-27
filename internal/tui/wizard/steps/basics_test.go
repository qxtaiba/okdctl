package steps

import (
	"reflect"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

func TestBasicsApplyRekeysNodeOverridesToTheNewClusterName(t *testing.T) {
	cfg := newPlacementConfig()
	cfg.Nodes = map[string]config.NodeOverride{
		"lab-worker1": {Host: "pve3"},
		"lab-master0": {CPU: new(8)},
		"other":       {Host: "pve1"},
	}
	step := NewBasicsStep()
	step.LoadFromConfig(cfg, true)
	step.SetValue(config.FieldClusterName, "prod")

	if err := step.Apply(cfg); err != nil {
		t.Fatal(err)
	}
	want := map[string]config.NodeOverride{
		"prod-worker1": {Host: "pve3"},
		"prod-master0": {CPU: new(8)},
		"other":        {Host: "pve1"},
	}
	if !reflect.DeepEqual(cfg.Nodes, want) {
		t.Fatalf("Nodes after rename = %+v, want %+v", cfg.Nodes, want)
	}
}

func TestResourcesApplyPrunesWorkerHostOverridesPastTheCount(t *testing.T) {
	cfg := newPlacementConfig()
	cfg.Nodes = map[string]config.NodeOverride{
		"lab-worker1": {Host: "pve3"},
		"lab-worker0": {CPU: new(8)},
	}
	step, _ := NewResourcesStep(nil)
	step.LoadFromConfig(cfg, true)
	step.SetValue(fieldWorkerCount, "0")

	if err := step.Apply(cfg); err != nil {
		t.Fatal(err)
	}
	if _, stale := cfg.Nodes["lab-worker1"]; stale {
		t.Errorf("a host override for a worker past the count survived: %+v", cfg.Nodes)
	}
	if o, kept := cfg.Nodes["lab-worker0"]; !kept || o.CPU == nil {
		t.Errorf("a non-host override was dropped instead of left for Validate: %+v", cfg.Nodes)
	}
}
