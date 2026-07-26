package lifecycle

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func TestParamsStepResizeApplyAndValidation(t *testing.T) {
	cfg := config.DefaultConfig()
	st := &State{Cfg: cfg, Op: node.OpResize, Scope: node.ResizeScope{Role: nodetypes.RoleMaster}}
	s := NewParamsStep(st)
	_ = s.Init()

	s.memField.SetValue("0")
	s.cpuField.SetValue("0")
	if err := s.Validate(); err == nil {
		t.Error("memory and cpu both zero must fail validation")
	}
	s.memField.SetValue("24576")
	s.timeoutField.SetValue("bogus")
	if err := s.Validate(); err == nil {
		t.Error("unparseable drain timeout must fail validation")
	}
	s.timeoutField.SetValue("10m")
	if err := s.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if st.MemoryMB != 24576 || st.CPU != 0 || st.SkipDrain || st.DrainTimeout != "10m" {
		t.Errorf("state = mem %d cpu %d skip %v timeout %q", st.MemoryMB, st.CPU, st.SkipDrain, st.DrainTimeout)
	}
}

func TestParamsStepSkipDrainSelection(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize, Scope: node.ResizeScope{Role: nodetypes.RoleWorker}}
	s := NewParamsStep(st)
	_ = s.Init()
	s.memField.SetValue("16384")
	s.drainModeField.SetValue(drainModeSkip)
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if !st.SkipDrain {
		t.Error("skip-drain selection must set state")
	}
	if !strings.Contains(s.View(90, 40), "power-cycled without evacuating pods") {
		t.Error("skip-drain must render its warning note")
	}
}

func TestParamsStepAddCount(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpAdd}
	s := NewParamsStep(st)
	_ = s.Init()
	s.countField.SetValue("2")
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if st.Count != 2 {
		t.Errorf("Count = %d, want 2", st.Count)
	}
	s.countField.SetValue("0")
	if err := s.Validate(); err == nil {
		t.Error("count 0 must fail validation")
	}
}

func TestParamsStepRemoveForceStorage(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpRemove, Target: "homelab-worker2"}
	s := NewParamsStep(st)
	_ = s.Init()
	s.forceStorageField.SetValue("yes")
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if !st.ForceStorage {
		t.Error("force storage selection must set state")
	}
	if st.DrainTimeout != "10m" {
		t.Errorf("DrainTimeout default = %q, want 10m", st.DrainTimeout)
	}
}

func TestParamsStepShownOnResume(t *testing.T) {
	// The backend does not persist an interrupted op's parameters, so a
	// resume must re-collect them: hiding this step would hand Resize
	// zero sizing (refused) and Remove an unbounded drain timeout.
	cfg := config.DefaultConfig()
	st := &State{
		Cfg: cfg, Op: node.OpResize, Resume: true,
		Scope: node.ResizeScope{Node: "homelab-master0"},
	}
	s := NewParamsStep(st)
	if !s.ShouldShow(cfg) {
		t.Fatal("params must be shown on resume")
	}
	_ = s.Init()
	if s.resizeRole() != nodetypes.RoleMaster {
		t.Error("resume must infer the role from the marker target name")
	}
}
