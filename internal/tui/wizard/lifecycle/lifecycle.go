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
	// Ack arms the backend's --acknowledge-interrupted-op override: the
	// operator explicitly chose a different op over a stranded marker.
	Ack bool

	Nodes  []cluster.NodeDetail
	Scope  node.ResizeScope // resize target
	Target string           // remove target

	MemoryMB     int
	CPU          int
	Count        int
	SkipDrain    bool
	ForceStorage bool
	DrainTimeout string

	Plan      *node.OpPlan
	DryRunErr error
	// Proceed is true only after the operator chose execute on the preview
	// screen (and, for destructive plans, passed the typed confirmation).
	Proceed bool
	// Started marks that execution began (mutations may have happened);
	// Executed marks that the backend returned. The gap between them is an
	// interrupted run the CLI must report truthfully.
	Started  bool
	Executed bool

	Result  error
	Elapsed time.Duration
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
