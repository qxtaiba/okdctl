// Package lifecycle implements the Cluster Lifecycle wizard flow: steps
// that collect a node operation interactively and drive internal/node.Runner
// through its Confirm/Preview/Reporter seams.
package lifecycle

import (
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// StepIDs for the lifecycle flow's screens.
const (
	StepIDOp      wizard.StepID = "lifecycle-op"
	StepIDTarget  wizard.StepID = "lifecycle-target"
	StepIDParams  wizard.StepID = "lifecycle-params"
	StepIDPreview wizard.StepID = "lifecycle-preview"
	StepIDConfirm wizard.StepID = "lifecycle-confirm"
	StepIDExec    wizard.StepID = "lifecycle-exec"
	StepIDDone    wizard.StepID = "lifecycle-done"
)

// State is shared by pointer across all lifecycle steps — the role
// *config.Config plays for the configure wizard. Steps write into it from
// Apply; ShouldShow reads it (ignoring the cfg argument).
type State struct {
	Cfg    *config.Config
	Op     node.Op
	Marker *node.OpMarker
	Resume bool
	// Ack maps to --acknowledge-interrupted-op (operator overrode a stranded marker).
	Ack bool

	Nodes  []cluster.NodeDetail
	Scope  node.ResizeScope // resize target
	Target string           // remove target

	MemoryMB     int
	CPU          int
	OSDiskGB     int
	Count        int
	SkipDrain    bool
	ForceStorage bool
	DrainTimeout string

	Plan      *node.OpPlan
	DryRunErr error
	// Proceed is true only after the operator executes the preview (and,
	// for destructive plans, confirms by typed name).
	Proceed bool
	// Started marks execution began; Executed marks the backend returned —
	// the gap is an interrupted run.
	Started  bool
	Executed bool

	Result  error
	Elapsed time.Duration
}

// DiskOnly reports whether the collected resize params grow only the os
// disk — the live path with no cordon/drain/power-cycle.
func (st *State) DiskOnly() bool {
	return st.Op == node.OpResize && st.OSDiskGB > 0 && st.MemoryMB <= 0 && st.CPU <= 0
}

// ExecEvent is one progress event on the execution screen's feed: either a
// structured OnStep transition (Step set), a Reporter span (Desc set, Done
// marking the closing bracket), or the terminal event (Final).
type ExecEvent struct {
	Node  string
	Step  node.Step
	Desc  string
	Done  bool
	Took  time.Duration
	Err   error
	Final bool
}

// Hooks are the CLI-supplied closures the steps call into, so the
// lifecycle package never imports the cli package's assembly code.
type Hooks struct {
	ListNodes func() ([]cluster.NodeDetail, error)
	DryRun    func(st *State) (*node.OpPlan, error)
	Execute   func(st *State, events chan<- ExecEvent) error
	CancelOp  func()
}
