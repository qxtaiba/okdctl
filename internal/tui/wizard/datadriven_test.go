package wizard

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

const testValYes = "yes"

func testStepDefinition() *StepDefinition {
	return &StepDefinition{
		ID:           StepIDBasics,
		Title:        "test step",
		DisplayTitle: "test step",
		Sections: []SectionDefinition{
			{
				Title: "section one",
				Fields: []FieldDefinition{
					{
						Key:       "name",
						Label:     "name",
						Required:  true,
						ConfigSet: SetString(func(c *config.Config, v string) { c.Cluster.Name = v }),
						ConfigGet: GetString(func(c *config.Config) string { return c.Cluster.Name }),
					},
					{
						Key:       "count",
						Label:     "count",
						ConfigSet: SetInt(func(c *config.Config, v int) { c.Topology.ControlPlane.Count = v }),
						ConfigGet: GetInt(func(c *config.Config) int { return c.Topology.ControlPlane.Count }),
					},
					{
						Key:       "approve",
						Label:     "approve",
						Default:   "no",
						Type:      FieldTypeSelect,
						Options:   []string{"no", testValYes},
						ConfigSet: SetBool(func(c *config.Config, v bool) { c.Deployment.AutoApprove = v }),
						ConfigGet: func(c *config.Config) string {
							if c.Deployment.AutoApprove {
								return testValYes
							}
							return "no"
						},
					},
				},
			},
		},
		Validate: func(values map[string]string) error {
			if values["name"] == "forbidden" {
				return errors.New("name is forbidden")
			}
			return nil
		},
		Apply: func(step *DataDrivenStep, cfg *config.Config) error {
			cfg.Cluster.Domain = "applied." + step.Value("name")
			return nil
		},
		ShouldShow: func(cfg *config.Config) bool {
			return cfg.Distribution.Type != "skip-me"
		},
	}
}

func TestDataDrivenStep_ValueSetValue(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())

	if got := step.Value("name"); got != "" {
		t.Fatalf("Value(name) = %q, want empty before any set", got)
	}

	step.setValue("name", "cluster-a")
	if got := step.Value("name"); got != "cluster-a" {
		t.Fatalf("Value(name) after setValue = %q, want cluster-a", got)
	}

	// Unknown keys are a no-op, not a panic.
	step.setValue("does-not-exist", "x")
	if got := step.Value("does-not-exist"); got != "" {
		t.Fatalf("Value(unknown key) = %q, want empty", got)
	}
}

func TestDataDrivenStep_ValueInt(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())

	if got := step.ValueInt("count", 42); got != 42 {
		t.Fatalf("ValueInt(count) on empty field = %d, want fallback 42", got)
	}

	step.setValue("count", "9")
	if got := step.ValueInt("count", 42); got != 9 {
		t.Fatalf("ValueInt(count) = %d, want 9", got)
	}

	step.setValue("count", "not-a-number")
	if got := step.ValueInt("count", 42); got != 42 {
		t.Fatalf("ValueInt(count) with unparsable value = %d, want fallback 42", got)
	}
}

func TestDataDrivenStep_LoadFromConfig(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())
	cfg := &config.Config{}
	cfg.Cluster.Name = "loaded-cluster"
	cfg.Topology.ControlPlane.Count = 5
	cfg.Deployment.AutoApprove = true

	step.LoadFromConfig(cfg, true)

	if got := step.Value("name"); got != "loaded-cluster" {
		t.Errorf("Value(name) = %q, want loaded-cluster", got)
	}
	if got := step.ValueInt("count", -1); got != 5 {
		t.Errorf("ValueInt(count) = %d, want 5", got)
	}
	if got := step.Value("approve"); got != testValYes {
		t.Errorf("Value(approve) = %q, want yes", got)
	}
}

// blankableDefaultStepDefinition returns a single non-required text field
// with a Default, mirroring secretstore_op_connect_host's shape: a value
// the wizard offers as a starting point but that a real config may
// legitimately have cleared.
func blankableDefaultStepDefinition() *StepDefinition {
	return &StepDefinition{
		ID: StepIDBasics,
		Sections: []SectionDefinition{
			{
				Fields: []FieldDefinition{
					{
						Key:       "host",
						Label:     "connect host",
						Default:   "http://onepassword-connect:8080",
						ConfigSet: SetString(func(c *config.Config, v string) { c.Cluster.Domain = v }),
						ConfigGet: GetString(func(c *config.Config) string { return c.Cluster.Domain }),
					},
				},
			},
		},
	}
}

func TestDataDrivenStep_LoadFromConfig_RealConfigBlankSurvivesRoundTrip(t *testing.T) {
	step := NewDataDrivenStep(blankableDefaultStepDefinition())
	cfg := &config.Config{} // Cluster.Domain == "": a real, saved config the user intentionally blanked

	step.LoadFromConfig(cfg, true)

	if got := step.Value("host"); got != "" {
		t.Fatalf("Value(host) after loading a real config with a blanked field = %q, want empty", got)
	}
	field, ok := step.getField("host").(*components.InputField)
	if !ok || field.IsDefault() {
		t.Fatal("IsDefault() after loading a real, intentionally-blanked config = true, want false")
	}

	out := &config.Config{}
	if err := step.Apply(out); err != nil {
		t.Fatalf("Apply(): %v", err)
	}
	if out.Cluster.Domain != "" {
		t.Fatalf("Apply() wrote %q, want the blank preserved", out.Cluster.Domain)
	}
}

func TestDataDrivenStep_LoadFromConfig_FreshSeedKeepsDefault(t *testing.T) {
	step := NewDataDrivenStep(blankableDefaultStepDefinition())
	cfg := &config.Config{} // a synthetic seed (e.g. config.DefaultConfig gap), not a real saved file

	step.LoadFromConfig(cfg, false)

	if got := step.Value("host"); got != "http://onepassword-connect:8080" {
		t.Fatalf("Value(host) after a fresh seed = %q, want the constructed default", got)
	}
	field, ok := step.getField("host").(*components.InputField)
	if !ok || !field.IsDefault() {
		t.Fatal("IsDefault() after a fresh seed = false, want true (default survives)")
	}
}

func TestDataDrivenStep_Validate(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())
	step.setValue("name", "cluster-a")

	if err := step.Validate(); err != nil {
		t.Fatalf("Validate() with valid values: %v", err)
	}

	step.setValue("name", "forbidden")
	if err := step.Validate(); err == nil {
		t.Fatal("Validate() with forbidden name: want error, got nil")
	}
}

func TestDataDrivenStep_Apply(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())
	step.setValue("name", "cluster-a")
	step.setValue("count", "7")
	step.setValue("approve", testValYes)

	cfg := &config.Config{}
	if err := step.Apply(cfg); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}

	if cfg.Cluster.Name != "cluster-a" {
		t.Errorf("cfg.Cluster.Name = %q, want cluster-a", cfg.Cluster.Name)
	}
	if cfg.Topology.ControlPlane.Count != 7 {
		t.Errorf("cfg.Topology.ControlPlane.Count = %d, want 7", cfg.Topology.ControlPlane.Count)
	}
	if !cfg.Deployment.AutoApprove {
		t.Error("cfg.Deployment.AutoApprove = false, want true")
	}
	// Step-level Apply runs after field auto-binding, so step.Value already reflects it.
	if cfg.Cluster.Domain != "applied.cluster-a" {
		t.Errorf("cfg.Cluster.Domain = %q, want applied.cluster-a", cfg.Cluster.Domain)
	}
}

func TestDataDrivenStep_Apply_PropagatesConfigSetError(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())
	step.setValue("count", "not-a-number")

	cfg := &config.Config{}
	if err := step.Apply(cfg); err == nil {
		t.Fatal("Apply() with unparsable int field: want error, got nil")
	}
}

func TestDataDrivenStep_UpdateEnterValidatesThenCompletes(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())
	step.SetFocused(true)

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}

	// Required "name" is empty: enter must fail validation and emit an error, not advance.
	if _, cmd := step.Update(enter); cmd == nil {
		t.Fatal("Update(enter) with empty required field: want error cmd, got nil")
	} else if _, ok := firstErrorSetMsg(cmd); !ok {
		t.Fatal("Update(enter) with empty required field: want an ErrorSetMsg, got none")
	}

	step.setValue("name", "cluster-a")
	_, cmd := step.Update(enter)
	if cmd == nil {
		t.Fatal("Update(enter) with valid values: want completion cmd, got nil")
	}
	msg := cmd()
	complete, ok := msg.(StepCompleteMsg)
	if !ok {
		t.Fatalf("Update(enter) emitted %T, want StepCompleteMsg", msg)
	}
	if complete.StepID != StepIDBasics {
		t.Errorf("StepCompleteMsg.StepID = %q, want %q", complete.StepID, StepIDBasics)
	}

	// Definition-level Validate rejects "forbidden": enter must emit an error, not advance.
	step.setValue("name", "forbidden")
	_, cmd = step.Update(enter)
	if cmd == nil {
		t.Fatal("Update(enter) with definition-forbidden value: want error cmd, got nil")
	}
	if errMsg, ok := firstErrorSetMsg(cmd); !ok || errMsg.Error.Error() != "name is forbidden" {
		t.Fatalf("Update(enter) with definition-forbidden value = %#v, want ErrorSetMsg(name is forbidden)", cmd())
	}
}

// firstErrorSetMsg runs cmd, flattening batches, and returns the first
// ErrorSetMsg produced along with whether one was found.
func firstErrorSetMsg(cmd tea.Cmd) (ErrorSetMsg, bool) {
	if cmd == nil {
		return ErrorSetMsg{}, false
	}
	switch msg := cmd().(type) {
	case ErrorSetMsg:
		return msg, true
	case tea.BatchMsg:
		for _, c := range msg {
			if m, ok := firstErrorSetMsg(c); ok {
				return m, true
			}
		}
	}
	return ErrorSetMsg{}, false
}

func TestDataDrivenStep_EnterMarksAllTouchedAndFocusesFirstInvalid(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())
	step.SetFocused(true)

	_, cmd := step.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Update(enter) with an empty required field: want a cmd, got nil")
	}
	if !containsFocusChanged(cmd) {
		t.Fatal("Update(enter) with invalid fields did not emit FocusChangedMsg")
	}
	if errMsg, ok := firstErrorSetMsg(cmd); !ok || !errors.Is(errMsg.Error, errFixHighlighted) {
		t.Fatalf("Update(enter) with invalid fields = %#v, want ErrorSetMsg(errFixHighlighted)", cmd())
	}

	if got := step.form.FocusedField(); got != step.getField("name") {
		t.Fatalf("FocusedField() after enter = %v, want the first invalid field (name)", got)
	}

	view := tuitest.StripANSI(step.View(80, 24))
	if !strings.Contains(view, "this field is required") {
		t.Fatalf("View() after enter = %q, want the required-field error visible", view)
	}
}

func TestBuildFormField_DefaultIsRealValue(t *testing.T) {
	def := &FieldDefinition{Key: "name", Label: "name", Default: "mycluster"}

	field := buildFormField(def)

	if got := field.Value(); got != "mycluster" {
		t.Fatalf("Value() before any input = %q, want the default %q", got, "mycluster")
	}
	inputField, ok := field.(*components.InputField)
	if !ok || !inputField.IsDefault() {
		t.Fatal("buildFormField with a Default did not mark the field IsDefault")
	}
}

func TestDataDrivenStep_ApplyWithDefaultsSucceeds(t *testing.T) {
	def := &StepDefinition{
		ID:    StepIDBasics,
		Title: "defaults step",
		Sections: []SectionDefinition{
			{
				Title: "section",
				Fields: []FieldDefinition{
					{
						Key:       "name",
						Label:     "name",
						Default:   "mycluster",
						Required:  true,
						ConfigSet: SetString(func(c *config.Config, v string) { c.Cluster.Name = v }),
					},
				},
			},
		},
	}
	step := NewDataDrivenStep(def)

	if err := step.Validate(); err != nil {
		t.Fatalf("Validate() with an untouched default value: %v", err)
	}

	cfg := &config.Config{}
	if err := step.Apply(cfg); err != nil {
		t.Fatalf("Apply() with an untouched default value: %v", err)
	}
	if cfg.Cluster.Name != "mycluster" {
		t.Fatalf("cfg.Cluster.Name = %q, want the default mycluster", cfg.Cluster.Name)
	}
}

func TestFormSection_IsCompleteHasNoSideEffects(t *testing.T) {
	field := components.NewInputField("name", "")
	field.Required = true
	section := FormSection{Group: components.NewInputGroup(field)}
	section.Group.SetWidth(60)

	if section.isComplete() {
		t.Fatal("isComplete() with an empty required field = true, want false")
	}

	errColor := lipgloss.NewStyle().Foreground(tui.ColorError).Render("x")
	prefix := errColor[:strings.IndexByte(errColor, 'x')]
	if strings.Contains(field.View(), prefix) {
		t.Fatal("View() after isComplete() carries ColorError styling, want no side effect")
	}
}

func TestDataDrivenStep_EnterDefinitionErrorEmitsErrorSetMsg(t *testing.T) {
	def := testStepDefinition()
	def.Validate = func(map[string]string) error { return errors.New("name is forbidden") }
	s := NewDataDrivenStep(def)
	s.SetFocused(true)
	s.setValue("name", "cluster-a") // required field must pass before definition-level Validate runs

	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a cmd")
	}
	if msg, ok := cmd().(ErrorSetMsg); !ok || msg.Error.Error() != "name is forbidden" {
		t.Fatalf("msg = %#v", cmd())
	}
}

func TestDataDrivenStep_ShortHelpIncludesFieldHints(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())
	step.SetFocused(true)

	base := step.ShortHelp()
	for _, kb := range base {
		if kb.Key == "space" || kb.Key == "←/→" {
			t.Fatalf("ShortHelp() with a text field focused = %+v, want no multi-select hints", base)
		}
	}

	def := &StepDefinition{
		ID:    StepIDBasics,
		Title: "test step",
		Sections: []SectionDefinition{
			{
				Title: "section one",
				Fields: []FieldDefinition{
					{Key: "networks", Label: "additional networks", Type: FieldTypeMultiSelect, Options: []string{"vmbr0", "vmbr1"}},
				},
			},
		},
	}
	multiStep := NewDataDrivenStep(def)
	multiStep.SetFocused(true)

	help := multiStep.ShortHelp()
	want := map[string]bool{"space": false, "←/→": false}
	for _, kb := range help {
		if _, ok := want[kb.Key]; ok {
			want[kb.Key] = true
		}
	}
	for k, found := range want {
		if !found {
			t.Errorf("ShortHelp() = %+v, want a hint for key %q", help, k)
		}
	}
	if len(help) != len(base)+2 {
		t.Errorf("ShortHelp() len = %d, want base %d + 2 field hints", len(help), len(base))
	}
}

func TestDataDrivenStep_ShouldShow(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())
	cfg := &config.Config{}

	if !step.ShouldShow(cfg) {
		t.Error("ShouldShow() = false, want true for default config")
	}

	cfg.Distribution.Type = "skip-me"
	if step.ShouldShow(cfg) {
		t.Error("ShouldShow() = true, want false when the predicate rejects")
	}
}

func TestDataDrivenStep_ShouldShow_DefaultsToTrue(t *testing.T) {
	def := &StepDefinition{ID: StepIDBasics}
	step := NewDataDrivenStep(def)
	if !step.ShouldShow(&config.Config{}) {
		t.Error("ShouldShow() with nil predicate = false, want true")
	}
}

func TestSetIntSetBool(t *testing.T) {
	cfg := &config.Config{}

	setInt := SetInt(func(c *config.Config, v int) { c.Topology.ControlPlane.Count = v })
	if err := setInt(cfg, "5"); err != nil {
		t.Fatalf("SetInt: %v", err)
	}
	if cfg.Topology.ControlPlane.Count != 5 {
		t.Errorf("SetInt did not set value: got %d", cfg.Topology.ControlPlane.Count)
	}
	if err := setInt(cfg, "not-an-int"); err == nil {
		t.Fatal("SetInt with non-numeric input: want error, got nil")
	}

	setBool := SetBool(func(c *config.Config, v bool) { c.Deployment.AutoApprove = v })
	for _, truthy := range []string{testValYes, "true", "1", "y", "YES"} {
		cfg.Deployment.AutoApprove = false
		if err := setBool(cfg, truthy); err != nil {
			t.Fatalf("SetBool(%q): %v", truthy, err)
		}
		if !cfg.Deployment.AutoApprove {
			t.Errorf("SetBool(%q) did not set true", truthy)
		}
	}
	for _, falsy := range []string{"no", ""} {
		cfg.Deployment.AutoApprove = true
		if err := setBool(cfg, falsy); err != nil {
			t.Fatalf("SetBool(%q): %v", falsy, err)
		}
		if cfg.Deployment.AutoApprove {
			t.Errorf("SetBool(%q) did not set false", falsy)
		}
	}
}

func TestFieldWidth_Cols(t *testing.T) {
	cases := []struct {
		w     FieldWidth
		avail int
		want  int
	}{
		{FieldWidthAuto, 90, 32},
		{FieldWidthNumber, 90, 12},
		{FieldWidthPath, 90, 56},
		{FieldWidthFull, 90, 90},
		{FieldWidthPath, 50, 50},
	}
	for _, c := range cases {
		if got := c.w.Cols(c.avail); got != c.want {
			t.Errorf("FieldWidth(%d).Cols(%d) = %d, want %d", c.w, c.avail, got, c.want)
		}
	}
}

func newSpanTestForm() *MultiSectionForm {
	return NewMultiSectionForm([]FormSection{
		{
			Title: "section one",
			Note:  "a note",
			Group: components.NewInputGroup(
				components.NewInputField("name", "cluster"),
				components.NewInputField("domain", "example.com"),
				components.NewSelectField("approve", []string{"no", testValYes}),
			),
		},
		{
			Title: "section two",
			Group: components.NewInputGroup(
				components.NewInputField("gateway", "10.0.0.1"),
				components.NewInputField("bastion", "10.0.0.2"),
				components.NewSelectField("mode", []string{"a", "b"}),
			),
		},
	})
}

func TestMultiSectionForm_SpansCoverEveryFieldWithoutOverlap(t *testing.T) {
	f := newSpanTestForm()
	lines := strings.Split(f.View(80), "\n")

	prev := -1
	for si := range f.spans {
		for fi, sp := range f.spans[si] {
			if sp.Start <= prev {
				t.Fatalf("span %d/%d starts at %d, not after %d", si, fi, sp.Start, prev)
			}
			if fi > 0 && sp.Start-prev != 2 {
				t.Fatalf("gap before span %d/%d is %d rows, want 1 blank row", si, fi, sp.Start-prev-1)
			}
			if got, want := sp.End-sp.Start+1, lipgloss.Height(f.FieldAt(si, fi).View()); got != want {
				t.Fatalf("span %d/%d is %d rows, want %d", si, fi, got, want)
			}
			if strings.TrimSpace(lines[sp.Start]) == "" {
				t.Fatalf("span %d/%d starts on a blank line", si, fi)
			}
			if strings.TrimSpace(lines[sp.End]) == "" {
				t.Fatalf("span %d/%d ends on a blank line", si, fi)
			}
			if strings.TrimSpace(lines[sp.Start-1]) != "" {
				t.Fatalf("row above span %d/%d is not blank: %q", si, fi, lines[sp.Start-1])
			}
			if strings.TrimSpace(lines[sp.End+1]) != "" {
				t.Fatalf("row below span %d/%d is not blank: %q", si, fi, lines[sp.End+1])
			}
			prev = sp.End
		}
	}
}

func TestMultiSectionForm_FocusedSpanFollowsFocus(t *testing.T) {
	f := newSpanTestForm()
	_ = f.Focus()
	_ = f.View(80)

	span, ok := f.FocusedSpan()
	if !ok || span != f.spans[0][0] {
		t.Fatalf("FocusedSpan() = %+v, %v; want %+v", span, ok, f.spans[0][0])
	}

	tab := tea.KeyPressMsg{Code: tea.KeyTab}
	for range 3 {
		f.Update(tab)
	}
	_ = f.View(80)

	span, ok = f.FocusedSpan()
	if !ok || span != f.spans[1][0] {
		t.Fatalf("after 3 tabs FocusedSpan() = %+v, %v; want %+v", span, ok, f.spans[1][0])
	}
}

func TestMultiSectionForm_TabEmitsFocusChanged(t *testing.T) {
	f := newSpanTestForm()
	_ = f.Focus()

	cmd, _ := f.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !containsFocusChanged(cmd) {
		t.Fatal("tab did not emit FocusChangedMsg")
	}
}

// containsFocusChanged runs cmd, flattening batches, and reports whether any
// resulting message is a FocusChangedMsg.
func containsFocusChanged(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case FocusChangedMsg:
		return true
	case tea.BatchMsg:
		for _, c := range msg {
			if containsFocusChanged(c) {
				return true
			}
		}
	}
	return false
}

func TestMultiSectionForm_SectionsJoinedByOneBlankRow(t *testing.T) {
	f := newSpanTestForm()
	lines := strings.Split(f.View(80), "\n")

	lastFieldEnd := f.spans[0][len(f.spans[0])-1].End
	if strings.TrimSpace(lines[lastFieldEnd+1]) != "" {
		t.Fatalf("row after section one's last field is not blank: %q", lines[lastFieldEnd+1])
	}
	if strings.TrimSpace(lines[lastFieldEnd+2]) == "" {
		t.Fatalf("row two after section one's last field is blank, want section two's head")
	}
	next := strings.TrimSpace(lines[lastFieldEnd+2])
	if !strings.Contains(next, "○") {
		t.Fatalf("row after the blank gap = %q, want the pending indicator ○", next)
	}
}

func TestMultiSectionForm_NoteWrapsToWidth(t *testing.T) {
	f := NewMultiSectionForm([]FormSection{
		{
			Title: "section one",
			Note:  strings.Repeat("a", 150),
			Group: components.NewInputGroup(components.NewInputField("name", "cluster")),
		},
	})

	for _, line := range strings.Split(f.View(50), "\n") {
		if got := lipgloss.Width(line); got > 50 {
			t.Fatalf("line %q is %d columns wide, want <= 50", line, got)
		}
	}
}

func TestMultiSectionForm_SectionWarningRendersUnderFields(t *testing.T) {
	f := NewMultiSectionForm([]FormSection{
		{
			Title:   "section one",
			Group:   components.NewInputGroup(components.NewInputField("name", "cluster")),
			Warning: func() string { return "something needs attention" },
		},
		{
			Title: "section two",
			Group: components.NewInputGroup(components.NewInputField("gateway", "10.0.0.1")),
		},
	})

	lines := strings.Split(f.View(80), "\n")
	lastFieldEnd := f.spans[0][len(f.spans[0])-1].End

	if strings.TrimSpace(lines[lastFieldEnd+1]) != "" {
		t.Fatalf("row after section one's last field is not blank: %q", lines[lastFieldEnd+1])
	}
	warningLine := strings.TrimSpace(lines[lastFieldEnd+2])
	if !strings.Contains(warningLine, tui.IconWarning) || !strings.Contains(warningLine, "something needs attention") {
		t.Fatalf("row after the blank gap = %q, want the warning block", warningLine)
	}
	if strings.TrimSpace(lines[lastFieldEnd+3]) != "" {
		t.Fatalf("row after the warning is not blank: %q", lines[lastFieldEnd+3])
	}
	if !strings.Contains(strings.TrimSpace(lines[lastFieldEnd+4]), "section two") {
		t.Fatalf("row after the warning's blank gap = %q, want section two's head", lines[lastFieldEnd+4])
	}
}

func TestRenderInfoCard_FitsWidth(t *testing.T) {
	for _, width := range []int{50, 70, 110} {
		body := lipgloss.Wrap(strings.Repeat("resource totals go here ", 10), width-4, "")
		card := RenderInfoCard("totals", body, width)
		for i, line := range strings.Split(card, "\n") {
			if got := lipgloss.Width(line); got != width {
				t.Errorf("width %d: row %d = %d columns, want %d", width, i, got, width)
			}
		}
	}
}
