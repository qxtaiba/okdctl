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
	s.timeoutField.SetValue(defaultDrainTimeout)
	if err := s.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if st.MemoryMB != 24576 || st.CPU != 0 || st.SkipDrain || st.DrainTimeout != defaultDrainTimeout {
		t.Errorf("state = mem %d cpu %d skip %v timeout %q", st.MemoryMB, st.CPU, st.SkipDrain, st.DrainTimeout)
	}
}

func TestParamsStepResizeDiskField(t *testing.T) {
	cfg := config.DefaultConfig() // master DiskGB = 50
	st := &State{Cfg: cfg, Op: node.OpResize, Scope: node.ResizeScope{Role: nodetypes.RoleMaster}}
	s := NewParamsStep(st)
	_ = s.Init()

	if s.diskField == nil {
		t.Fatal("resize form must have an os disk field")
	}

	// Cross-field rule: only disk set (memory and cpu stay at zero) must
	// still pass — disk is a valid resize dimension on its own.
	s.memField.SetValue("0")
	s.cpuField.SetValue("0")
	s.diskField.SetValue("100")
	if err := s.Validate(); err != nil {
		t.Fatalf("disk-only resize must validate: %v", err)
	}
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if st.OSDiskGB != 100 {
		t.Errorf("OSDiskGB = %d, want 100", st.OSDiskGB)
	}
	if !st.DiskOnly() {
		t.Error("state must report DiskOnly after a disk-only apply")
	}
}

func TestParamsStepDiskGrowOnlyRefusesShrinkOrEqual(t *testing.T) {
	cfg := config.DefaultConfig() // master DiskGB = 50
	st := &State{Cfg: cfg, Op: node.OpResize, Scope: node.ResizeScope{Role: nodetypes.RoleMaster}}
	s := NewParamsStep(st)
	_ = s.Init()

	s.memField.SetValue("0")
	s.cpuField.SetValue("0")
	s.diskField.SetValue("50") // equal to current — grow-only refuses it
	if err := s.Validate(); err == nil {
		t.Error("disk value equal to current must fail validation")
	}
	s.diskField.SetValue("40") // below current — grow-only refuses it
	if err := s.Validate(); err == nil {
		t.Error("disk value below current must fail validation")
	}
	s.diskField.SetValue("60") // above current — allowed
	if err := s.Validate(); err != nil {
		t.Errorf("disk value above current must validate: %v", err)
	}
}

func TestParamsStepRefusesOSDiskGBOnSingleNodeTarget(t *testing.T) {
	cfg := config.DefaultConfig() // master DiskGB = 50
	st := &State{Cfg: cfg, Op: node.OpResize, Scope: node.ResizeScope{Node: "master0"}}
	s := NewParamsStep(st)
	_ = s.Init()

	s.memField.SetValue("0")
	s.cpuField.SetValue("0")
	s.diskField.SetValue("100")
	err := s.Validate()
	if err == nil {
		t.Fatal("disk grow against a single-node target must fail validation")
	}
	if !strings.Contains(err.Error(), "role-scoped") {
		t.Errorf("error must explain disk resizes are role-scoped, got %v", err)
	}

	// memory/cpu against the same single-node target is unaffected.
	s.diskField.SetValue("0")
	s.memField.SetValue("24576")
	if err := s.Validate(); err != nil {
		t.Errorf("single-node memory resize must validate: %v", err)
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
	if st.DrainTimeout != defaultDrainTimeout {
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

func TestParamsStepRebuildsFormWhenOpChanges(t *testing.T) {
	// The step instance survives esc-back-and-repick; a cached form for a
	// different op used to Apply through nil field pointers and panic.
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpAdd}
	s := NewParamsStep(st)
	_ = s.Init()
	if s.countField == nil {
		t.Fatal("add form must have a count field")
	}

	st.Op = node.OpRemove
	_ = s.Init()
	if s.drainModeField == nil || s.forceStorageField == nil {
		t.Fatal("op change must rebuild the form for the new op")
	}
	if s.countField != nil {
		t.Error("stale add fields must be cleared on rebuild")
	}
	if err := s.Apply(nil); err != nil { // used to nil-pointer panic
		t.Fatalf("Apply after op change: %v", err)
	}
	if st.DrainTimeout != defaultDrainTimeout {
		t.Errorf("DrainTimeout = %q, want the remove default", st.DrainTimeout)
	}
}
