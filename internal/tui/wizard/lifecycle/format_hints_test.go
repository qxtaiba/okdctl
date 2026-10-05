package lifecycle

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
)

// TestParamsStep_HelpStatesShape asserts that the params step's format-shaped
// fields — memory (a hard okd-minimum floor its validator enforces) and
// drain timeout (a Go duration format its validator enforces) — state that
// shape in their focused Help row, and that the add-op count field notes
// its validator's floor since the wording reads naturally.
func TestParamsStep_HelpStatesShape(t *testing.T) {
	resize := NewParamsStep(&State{Cfg: config.DefaultConfig(), Op: node.OpResize})
	_ = resize.Init()
	if !strings.Contains(resize.memField.Help, "okd minimum: 8192 mb") {
		t.Errorf("resize memField.Help = %q, want it to state the okd minimum floor", resize.memField.Help)
	}
	if !strings.Contains(resize.timeoutField.Help, "a duration like 10m or 1h") {
		t.Errorf("resize timeoutField.Help = %q, want it to state the duration shape", resize.timeoutField.Help)
	}

	remove := NewParamsStep(&State{Cfg: config.DefaultConfig(), Op: node.OpRemove})
	_ = remove.Init()
	if !strings.Contains(remove.timeoutField.Help, "a duration like 10m or 1h") {
		t.Errorf("remove timeoutField.Help = %q, want it to state the duration shape", remove.timeoutField.Help)
	}

	add := NewParamsStep(&State{Cfg: config.DefaultConfig(), Op: node.OpAdd})
	_ = add.Init()
	if !strings.Contains(add.countField.Help, "at least 1") {
		t.Errorf("add countField.Help = %q, want it to state the validator's floor", add.countField.Help)
	}
}
