package lifecycle

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

// Drain-mode option wording carries the semantics, informing the choice at selection time.
const (
	drainModeDefault = "cordon + drain (default)"
	drainModeSkip    = "skip drain — restart pods in place"

	// sectionDisruption titles the drain/timeout section and the matching preview entry.
	sectionDisruption = "disruption"

	defaultDrainTimeout = "10m"
	// okdMinMemoryMB mirrors the resources step's OKD minimum for node memory.
	okdMinMemoryMB = 8192
)

// ParamsStep collects the per-op parameters: memory/cpu and drain mode for
// resize, count for add, drain mode and force-storage for remove.
type ParamsStep struct {
	wizard.BaseStep
	st    *State
	inner *wizard.MultiSectionForm
	// builtFor guards against a stale form: esc-back-and-repick would
	// otherwise validate/Apply through nil pointers for the wrong op.
	builtFor node.Op

	memField          *components.InputField
	cpuField          *components.InputField
	diskField         *components.InputField
	countField        *components.InputField
	timeoutField      *components.InputField
	drainModeField    *components.SelectField
	forceStorageField *components.SelectField
}

// NewParamsStep constructs the parameter form step; the form itself is
// built lazily in Init, once the op selection is known.
func NewParamsStep(st *State) *ParamsStep {
	return &ParamsStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(StepIDParams,
			"parameters", "operation parameters", ""),
		st: st,
	}
}

// ShouldShow always shows the step: an interrupted op's parameters are not
// persisted, so resume must re-collect them (resize refuses zero sizing,
// remove would drain unbounded).
func (s *ParamsStep) ShouldShow(_ *config.Config) bool {
	return true
}

// Init builds the per-op form on first focus — rebuilding whenever the
// operation changed since the last visit — and focuses it.
func (s *ParamsStep) Init() tea.Cmd {
	s.ensureForm()
	return s.inner.Init()
}

func (s *ParamsStep) ensureForm() {
	if s.inner == nil || s.builtFor != s.st.Op {
		s.buildForm()
	}
}

func (s *ParamsStep) buildForm() {
	s.builtFor = s.st.Op
	s.memField, s.cpuField, s.diskField, s.countField, s.timeoutField = nil, nil, nil, nil, nil
	s.drainModeField, s.forceStorageField = nil, nil

	var sections []wizard.FormSection
	switch s.st.Op {
	case node.OpAdd:
		s.countField = components.NewInputField("workers to add", "1")
		s.countField.Help = "number of workers created in this batch"
		s.countField.SetValue("1")
		s.countField.Validator = validatePositiveInt
		sections = append(sections, wizard.FormSection{
			Title: "workers",
			Group: components.NewInputGroup(s.countField),
		})
	case node.OpRemove:
		s.buildDisruptionFields()
		s.forceStorageField = components.NewSelectField("force storage", []string{"no", "yes"})
		s.forceStorageField.Help = "allow removal when the worker holds rook-ceph osds — destroys their data disk"
		s.forceStorageField.SetDefault("no")
		sections = append(sections, wizard.FormSection{
			Title: sectionDisruption,
			Group: components.NewInputGroup(s.drainModeField, s.timeoutField, s.forceStorageField),
		})
	default: // resize
		current := s.st.Cfg.Topology.Workers
		if s.resizeRole() == nodetypes.RoleMaster {
			current = s.st.Cfg.Topology.ControlPlane
		}
		s.memField = components.NewInputField("memory (mb)", strconv.Itoa(current.MemoryMB))
		s.memField.Help = fmt.Sprintf("per-node memory — current: %d, 0 keeps current", current.MemoryMB)
		s.memField.Validator = validateMemoryMB
		s.cpuField = components.NewInputField("vcpus", strconv.Itoa(current.CPU))
		s.cpuField.Help = fmt.Sprintf("per-node cpu cores — current: %d, 0 keeps current", current.CPU)
		s.cpuField.SetValue("0")
		s.cpuField.Validator = validateNonNegativeInt
		s.diskField = components.NewInputField("os disk (gb)", strconv.Itoa(current.DiskGB))
		s.diskField.Help = fmt.Sprintf("per-node os disk — current: %d, grow-only, 0 keeps current; disk-only resizes are live (no power-cycle)", current.DiskGB)
		s.diskField.SetValue("0")
		s.diskField.Validator = validateNonNegativeInt
		sections = append(sections, wizard.FormSection{
			Title: "sizing",
			Group: components.NewInputGroup(s.memField, s.cpuField, s.diskField),
		})
		s.buildDisruptionFields()
		sections = append(sections, wizard.FormSection{
			Title: sectionDisruption,
			Group: components.NewInputGroup(s.drainModeField, s.timeoutField),
		})
	}
	s.inner = wizard.NewMultiSectionForm(sections)
}

func (s *ParamsStep) buildDisruptionFields() {
	s.drainModeField = components.NewSelectField("drain mode", []string{drainModeDefault, drainModeSkip})
	s.drainModeField.Help = "how pods leave the node"
	s.drainModeField.SetDefault(drainModeDefault)
	s.timeoutField = components.NewInputField("drain timeout", defaultDrainTimeout)
	s.timeoutField.Help = "per-node"
	s.timeoutField.SetValue(defaultDrainTimeout)
	s.timeoutField.Validator = validateDuration
}

func (s *ParamsStep) resizeRole() nodetypes.NodeRole {
	if s.st.Scope.Role != "" {
		return s.st.Scope.Role
	}
	for _, n := range s.st.Nodes {
		if n.Name == s.st.Scope.Node {
			return n.Role
		}
	}
	// Resume has no live list; the node name itself carries the role (<cluster>-masterN).
	if strings.Contains(s.st.Scope.Node, "master") {
		return nodetypes.RoleMaster
	}
	return nodetypes.RoleWorker
}

// Validate enforces the cross-field rule the flag verbs enforce: a resize
// needs at least one of memory/cpu, and every populated field must pass its
// own validator.
func (s *ParamsStep) Validate() error {
	for _, f := range s.fields() {
		if err := f.Validate(); err != nil {
			return err
		}
	}
	if s.st.Op == node.OpResize {
		if intValue(s.memField) <= 0 && intValue(s.cpuField) <= 0 && intValue(s.diskField) <= 0 {
			return &errtypes.UsageError{Msg: "resize requires at least one of memory, vcpus, or os disk"}
		}
		if d := intValue(s.diskField); d > 0 {
			if d <= s.currentDiskGB() {
				return &errtypes.UsageError{Msg: fmt.Sprintf("os disk is grow-only: %d must exceed the current %d GiB", d, s.currentDiskGB())}
			}
			// Mirrors the runner-level refusal in node.Resize: disk sizing
			// persists role-wide, but a single-node grow leaves same-role
			// siblings unable to ever catch up (CoreOS grows the filesystem
			// on firstboot only), so it is caught here too rather than only
			// surfacing after the wizard's dry-run preview runs.
			if s.st.Scope.Node != "" {
				return &errtypes.UsageError{Msg: "os disk is role-scoped: pick 'masters' or 'workers' as the target, not a single node"}
			}
		}
	}
	return nil
}

// currentDiskGB resolves the resize-scoped role's current os disk size,
// mirroring the "current" lookup buildForm uses for the field defaults.
func (s *ParamsStep) currentDiskGB() int {
	if s.resizeRole() == nodetypes.RoleMaster {
		return s.st.Cfg.Topology.ControlPlane.DiskGB
	}
	return s.st.Cfg.Topology.Workers.DiskGB
}

func (s *ParamsStep) fields() []components.FormField {
	var out []components.FormField
	for _, f := range []*components.InputField{s.memField, s.cpuField, s.diskField, s.countField, s.timeoutField} {
		if f != nil {
			out = append(out, f)
		}
	}
	for _, f := range []*components.SelectField{s.drainModeField, s.forceStorageField} {
		if f != nil {
			out = append(out, f)
		}
	}
	return out
}

// Update forwards input to the form; enter validates and completes.
func (s *ParamsStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	if s.inner == nil {
		return s, nil
	}
	cmd, enterPressed := s.inner.Update(msg)
	if !enterPressed {
		return s, cmd
	}
	if err := s.Validate(); err != nil {
		return s, func() tea.Msg { return wizard.ErrorSetMsg{Error: err} }
	}
	return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDParams} }
}

// View renders the form plus the amber skip-drain note when selected.
func (s *ParamsStep) View(width, height int) string {
	s.SetSize(width, height)
	s.ensureForm()
	out := s.inner.View(width)
	if s.drainModeField != nil && s.drainModeField.Value() == drainModeSkip {
		warn := lipgloss.NewStyle().Foreground(tui.ColorWarning).PaddingLeft(2)
		out += "\n" + warn.Render(strings.Join([]string{
			"⚠ skip-drain: the node is power-cycled without evacuating pods —",
			"  they die with the vm and restart in place on the resized node.",
			"  use when a memory-saturated cluster cannot reschedule evictions.",
			"  the etcd and ceph health gates still run.",
		}, "\n"))
	}
	return out
}

// Apply writes the collected parameters into the shared state.
func (s *ParamsStep) Apply(_ *config.Config) error {
	if err := s.Validate(); err != nil {
		return err
	}
	switch s.st.Op {
	case node.OpAdd:
		s.st.Count = intValue(s.countField)
	case node.OpRemove:
		s.st.SkipDrain = s.drainModeField.Value() == drainModeSkip
		s.st.DrainTimeout = s.timeoutField.Value()
		s.st.ForceStorage = s.forceStorageField.Value() == "yes"
	default:
		s.st.MemoryMB = intValue(s.memField)
		s.st.CPU = intValue(s.cpuField)
		s.st.OSDiskGB = intValue(s.diskField)
		s.st.SkipDrain = s.drainModeField.Value() == drainModeSkip
		s.st.DrainTimeout = s.timeoutField.Value()
	}
	return nil
}

// SetFocused propagates focus to the inner form.
func (s *ParamsStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if s.inner == nil {
		return
	}
	if focused {
		_ = s.inner.Focus() // command executed during Init
		return
	}
	s.inner.Blur()
}

// ShortHelp returns the form help bar.
func (s *ParamsStep) ShortHelp() []wizard.KeyBinding {
	return []wizard.KeyBinding{
		{Key: "↑↓/tab", Help: wizard.HelpNavigate},
		{Key: "← →", Help: "change value"},
		{Key: wizard.HelpEnter, Help: wizard.HelpContinue},
		{Key: wizard.HelpEsc, Help: wizard.HelpBack},
	}
}

func intValue(f *components.InputField) int {
	if f == nil {
		return 0
	}
	v, err := strconv.Atoi(strings.TrimSpace(f.Value()))
	if err != nil {
		return 0
	}
	return v
}

func validatePositiveInt(v string) error {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 {
		return errors.New("must be a whole number >= 1")
	}
	return nil
}

func validateNonNegativeInt(v string) error {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return errors.New("must be a whole number >= 0")
	}
	return nil
}

func validateMemoryMB(v string) error {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return errors.New("must be a whole number >= 0")
	}
	if n > 0 && n < okdMinMemoryMB {
		return fmt.Errorf("okd minimum is %d mb (or 0 to keep current)", okdMinMemoryMB)
	}
	return nil
}

func validateDuration(v string) error {
	if _, err := time.ParseDuration(strings.TrimSpace(v)); err != nil {
		return errors.New("must be a duration like 10m or 1h")
	}
	return nil
}
