package wizard

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/render"
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

func TestDataDrivenStep_Answered(t *testing.T) {
	def := testStepDefinition()
	def.Answered = func(values map[string]string) []render.Fact {
		return []render.Fact{{Key: "name", Value: values["name"]}}
	}
	step := NewDataDrivenStep(def)
	step.setValue("name", "homelab")

	facts := step.Answered()
	if len(facts) != 1 || facts[0] != (render.Fact{Key: "name", Value: "homelab"}) {
		t.Fatalf("Answered() = %+v", facts)
	}
}

func TestDataDrivenStep_AnsweredNilWhenDefinitionLeavesItUnset(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())
	if facts := step.Answered(); facts != nil {
		t.Fatalf("Answered() = %+v, want nil", facts)
	}
}

func TestDataDrivenStep_FieldHistoryUsesStableIDsAndExcludesCredentials(t *testing.T) {
	def := &StepDefinition{
		ID:    StepIDProxmox,
		Title: "history test",
		Sections: []SectionDefinition{{
			Title: "connection",
			Fields: []FieldDefinition{
				{Key: "host", Label: "host", Default: "pve-old"},
				{Key: "password", Label: "password", Type: FieldTypePassword},
			},
		}},
	}
	step := NewDataDrivenStep(def)
	step.View(60, 20)
	step.form.Focus()
	step.SetValue("host", "pve-current")
	step.form.Blur()
	step.form.Focus()

	step.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	field := step.getField("host").(*components.InputField)
	if !field.HistoryChooserOpen() || !strings.Contains(tuitest.StripANSI(field.View()), "pve-old") {
		t.Fatalf("field history was not reachable through the form: %s", tuitest.StripANSI(field.View()))
	}
	if safeFieldHistory(&FieldDefinition{Key: "password", Type: FieldTypePassword}) ||
		safeFieldHistory(&FieldDefinition{Key: "secretstore_vault_token"}) ||
		safeFieldHistory(&FieldDefinition{Key: "token_id"}) {
		t.Fatal("credential fields must not be eligible for history")
	}
}

func TestDataDrivenStep_FocusedFieldHelp(t *testing.T) {
	def := testStepDefinition()
	def.Sections[0].Fields[0].Help = "cluster name help"
	step := NewDataDrivenStep(def)
	_ = step.form.Focus()

	label, help, ok := step.FocusedFieldHelp()
	if !ok || label != "name" || help != "cluster name help" {
		t.Fatalf("FocusedFieldHelp() = %q, %q, %v", label, help, ok)
	}
}

func TestDataDrivenStep_FocusedFieldHelpFalseWithoutHelpText(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition()) // "name" field has no Help
	_ = step.form.Focus()

	if _, _, ok := step.FocusedFieldHelp(); ok {
		t.Fatal("FocusedFieldHelp() ok = true, want false for a field with no help text")
	}
}

func TestDataDrivenStep_FocusedFieldHelpFollowsFocus(t *testing.T) {
	def := testStepDefinition()
	def.Sections[0].Fields[0].Help = "name help"
	def.Sections[0].Fields[1].Help = "count help"
	step := NewDataDrivenStep(def)
	_ = step.form.Focus()

	if label, _, ok := step.FocusedFieldHelp(); !ok || label != "name" {
		t.Fatalf("initial focus = %q, %v", label, ok)
	}

	step.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	label, help, ok := step.FocusedFieldHelp()
	if !ok || label != "count" || help != "count help" {
		t.Fatalf("after tab = %q, %q, %v", label, help, ok)
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

func hiddenSectionStepDefinition() *StepDefinition {
	return &StepDefinition{
		ID:           StepIDBasics,
		Title:        "hidden section",
		DisplayTitle: "hidden section",
		Sections: []SectionDefinition{
			{
				Title: "visible section",
				Fields: []FieldDefinition{
					{
						Key:       "name",
						Label:     "name",
						ConfigSet: SetString(func(c *config.Config, v string) { c.Cluster.Name = v }),
					},
				},
			},
			{
				Title:   "hidden section",
				Visible: func(_ map[string]string) bool { return false },
				Fields: []FieldDefinition{
					{
						Key:       "secret",
						Label:     "secret",
						ConfigSet: SetString(func(c *config.Config, v string) { c.Cluster.Domain = v }),
					},
				},
			},
		},
	}
}

func TestDataDrivenStep_Apply_SkipsHiddenSection(t *testing.T) {
	step := NewDataDrivenStep(hiddenSectionStepDefinition())
	step.setValue("name", "cluster-a")
	step.setValue("secret", "stale-value")

	cfg := &config.Config{}
	if err := step.Apply(cfg); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}

	if cfg.Cluster.Name != "cluster-a" {
		t.Errorf("cfg.Cluster.Name = %q, want cluster-a", cfg.Cluster.Name)
	}
	if cfg.Cluster.Domain != "" {
		t.Errorf("cfg.Cluster.Domain = %q, want empty: hidden section's field must not be applied", cfg.Cluster.Domain)
	}
}

func TestDataDrivenStep_Values_ExcludesHiddenSection(t *testing.T) {
	def := hiddenSectionStepDefinition()
	var captured map[string]string
	def.Answered = func(values map[string]string) []render.Fact {
		captured = values
		return nil
	}
	step := NewDataDrivenStep(def)
	step.setValue("name", "cluster-a")
	step.setValue("secret", "stale-value")

	step.Answered()

	if _, ok := captured["secret"]; ok {
		t.Errorf("values()[secret] = %q, want key absent: hidden section's field must not appear", captured["secret"])
	}
	if captured["name"] != "cluster-a" {
		t.Errorf("values()[name] = %q, want cluster-a", captured["name"])
	}
}

func providerSwitchStepDefinition() *StepDefinition {
	return &StepDefinition{
		ID:           StepIDBasics,
		Title:        "provider switch",
		DisplayTitle: "provider switch",
		Sections: []SectionDefinition{
			{
				Title: "provider",
				Fields: []FieldDefinition{
					{
						Key:     "provider",
						Label:   "provider",
						Type:    FieldTypeSelect,
						Default: "a",
						Options: []string{"a", "b"},
					},
				},
			},
			{
				Title: "provider a",
				Visible: func(values map[string]string) bool {
					return values["provider"] == "a"
				},
				Fields: []FieldDefinition{
					{
						Key:       "field_a",
						Label:     "field a",
						ConfigSet: SetString(func(c *config.Config, v string) { c.Cluster.Name = v }),
					},
				},
			},
			{
				Title: "provider b",
				Visible: func(values map[string]string) bool {
					return values["provider"] == "b"
				},
				Fields: []FieldDefinition{
					{
						Key:       "field_b",
						Label:     "field b",
						ConfigSet: SetString(func(c *config.Config, v string) { c.Cluster.Domain = v }),
					},
				},
			},
		},
	}
}

func TestDataDrivenStep_Apply_ProviderSwitchDropsStaleSection(t *testing.T) {
	step := NewDataDrivenStep(providerSwitchStepDefinition())
	step.setValue("field_a", "typed-into-a")
	step.setValue("provider", "b")

	cfg := &config.Config{}
	if err := step.Apply(cfg); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}

	if cfg.Cluster.Name != "" {
		t.Errorf("cfg.Cluster.Name = %q, want empty: provider a's typed value must not persist after switching to provider b", cfg.Cluster.Name)
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
	if errMsg, ok := firstErrorSetMsg(cmd); !ok || !errors.Is(errMsg.Error, ErrFixHighlighted) {
		t.Fatalf("Update(enter) with invalid fields = %#v, want ErrorSetMsg(ErrFixHighlighted)", cmd())
	}

	if got := step.form.FocusedField(); got != step.getField("name") {
		t.Fatalf("FocusedField() after enter = %v, want the first invalid field (name)", got)
	}

	view := tuitest.StripANSI(step.View(80, 24))
	if !strings.Contains(view, "name is required") {
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

	errColor := lipgloss.NewStyle().Foreground(tui.ColorError()).Render("x")
	prefix := errColor[:strings.IndexByte(errColor, 'x')]
	if strings.Contains(field.View(), prefix) {
		t.Fatal("View() after isComplete() carries ColorError() styling, want no side effect")
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

// crossFieldTestDefinition models a networking-shaped overlap check: two
// fields whose cross-field Validate failure implicates the second one, the
// way machine_cidr/service_cidr overlap implicates service_cidr.
func crossFieldTestDefinition() *StepDefinition {
	return &StepDefinition{
		ID:    StepIDBasics,
		Title: "cross-field test step",
		Sections: []SectionDefinition{
			{
				Title: "section one",
				Fields: []FieldDefinition{
					{Key: "machine_cidr", Label: "machine cidr"},
					{Key: "service_cidr", Label: "service cidr"},
				},
			},
		},
		Validate: func(values map[string]string) error {
			if values["machine_cidr"] != "" && values["machine_cidr"] == values["service_cidr"] {
				return NewCrossFieldError(
					errors.New("machine cidr and service cidr must not overlap — widen or move one of the ranges"),
					"service_cidr",
				)
			}
			return nil
		},
	}
}

// TestDataDrivenStep_CrossFieldErrorFocusesAndMarksImplicatedField pins the
// reconciliation-audit defect where a cross-field failure (the CIDR-overlap
// shape) only ever reached the status-row banner: the implicated field
// never got focused, revealed, or marked invalid inline, so an operator
// with the field scrolled out of view saw a complaint with no indication
// of where.
func TestDataDrivenStep_CrossFieldErrorFocusesAndMarksImplicatedField(t *testing.T) {
	step := NewDataDrivenStep(crossFieldTestDefinition())
	step.SetFocused(true)
	step.setValue("machine_cidr", "10.0.0.0/16")
	step.setValue("service_cidr", "10.0.0.0/16")

	// Park focus on the first field — standing in for the implicated field
	// being scrolled out of view when enter is pressed.
	if cmd := step.form.FocusField(0, 0); cmd != nil {
		cmd()
	}

	_, cmd := step.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Update(enter) with overlapping cidrs: want a cmd, got nil")
	}
	if !containsFocusChanged(cmd) {
		t.Fatal("Update(enter) with a cross-field error did not emit FocusChangedMsg")
	}

	if got := step.form.FocusedField(); got != step.getField("service_cidr") {
		t.Fatalf("FocusedField() after a cross-field error = %v, want the implicated field (service_cidr)", got)
	}

	view := tuitest.StripANSI(step.View(80, 24))
	if !strings.Contains(view, "must not overlap") {
		t.Fatalf("View() after a cross-field error = %q, want the inline error visible", view)
	}

	// Correcting the value clears the stale inline error.
	if cmd := step.form.FocusField(0, 1); cmd != nil {
		cmd()
	}
	_, _ = step.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})

	fieldView := tuitest.StripANSI(step.getField("service_cidr").View())
	if strings.Contains(fieldView, "must not overlap") {
		t.Fatalf("field View() after editing = %q, still shows the stale cross-field error", fieldView)
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
		// Below either class's derived ceiling, the avail clamp wins —
		// unchanged from before the ceilings started tracking formMaxWidth.
		{FieldWidthAuto, 90, 90},
		{FieldWidthNumber, 90, 16},
		{FieldWidthPath, 90, 90},
		{FieldWidthFull, 90, 90},
		{FieldWidthPath, 50, 50},
		{FieldWidthAuto, 32, 32},
		{FieldWidthPath, 74, 74},
		// A generous avail (a wide split-layout form column) lets each
		// class's own derived ceiling — tied to formMaxWidth, not a bare
		// literal — win instead.
		{FieldWidthAuto, 1000, formMaxWidth - formFieldChrome},
		{FieldWidthPath, 1000, formMaxWidth - 4},
		{FieldWidthFull, 1000, 1000},
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
	if !strings.Contains(next, "section two") {
		t.Fatalf("row after the blank gap = %q, want section two's head", next)
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

func TestMultiSectionForm_HiddenSectionSkippedByViewAndNavigation(t *testing.T) {
	f := NewMultiSectionForm([]FormSection{
		{
			Title: "section one",
			Group: components.NewInputGroup(components.NewInputField("name", "cluster")),
		},
		{
			Title:   "section two (hidden)",
			Group:   components.NewInputGroup(components.NewInputField("gateway", "10.0.0.1")),
			Visible: func() bool { return false },
		},
		{
			Title: "section three",
			Group: components.NewInputGroup(components.NewInputField("domain", "example.com")),
		},
	})

	view := f.View(80)
	if strings.Contains(view, "section two (hidden)") {
		t.Fatalf("hidden section rendered:\n%s", view)
	}
	if !strings.Contains(view, "section one") || !strings.Contains(view, "section three") {
		t.Fatalf("visible sections missing from render:\n%s", view)
	}

	_ = f.Focus()
	if f.CurrentSection() != 0 {
		t.Fatalf("Focus() landed on section %d, want 0 (section one)", f.CurrentSection())
	}

	tab := tea.KeyPressMsg{Code: tea.KeyTab}
	f.Update(tab) // section one's only field is last, so tab must skip the hidden section
	if f.CurrentSection() != 2 {
		t.Fatalf("after tab past section one's last field, CurrentSection() = %d, want 2 (skipping the hidden section)", f.CurrentSection())
	}

	up := tea.KeyPressMsg{Code: tea.KeyUp} // bound the same as shift+tab
	f.Update(up)
	if f.CurrentSection() != 0 {
		t.Fatalf("after navigating back from section three, CurrentSection() = %d, want 0 (skipping the hidden section)", f.CurrentSection())
	}
}

// TestMultiSectionForm_EnterMidEditReachesTheField pins bug 4: enter while
// a KeyValueField is mid-edit must commit the cell edit, not submit the
// whole step — the form routes enter to a field that consumes it before
// matching its own submit binding.
func TestMultiSectionForm_EnterMidEditReachesTheField(t *testing.T) {
	kv := components.NewKeyValueField("tf env")
	form := NewMultiSectionForm([]FormSection{{
		Title: "env",
		Group: components.NewInputGroup(kv),
	}})
	_ = form.Init()
	_, _ = form.Update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	if !kv.ConsumesTextInput() {
		t.Fatal("ctrl+e did not enter edit mode")
	}

	_, enterPressed := form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if enterPressed {
		t.Fatal("enter mid-edit submitted the step, want it routed to the field")
	}
	if kv.ConsumesTextInput() {
		t.Fatal("enter mid-edit did not commit the edit (still in edit mode)")
	}

	_, enterPressed = form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !enterPressed {
		t.Fatal("enter in navigate mode must still submit the step")
	}
}

// TestFormSection_OptionalBlankFieldStillCompletes pins bug 36: a section
// whose only unfilled fields are optional (token id, vip, ntp server) shows
// the completed indicator instead of staying pending forever.
func TestFormSection_OptionalBlankFieldStillCompletes(t *testing.T) {
	optional := components.NewInputField("token id", "")
	required := components.NewInputField("host", "")
	required.Required = true
	section := FormSection{Group: components.NewInputGroup(required, optional)}

	if section.isComplete() {
		t.Fatal("section with an empty required field must be incomplete")
	}
	required.SetValue("pve.local")
	if !section.isComplete() {
		t.Fatal("section with only an optional blank left must be complete")
	}
}

// TestDataDrivenStep_LoadFromConfig_SelectOutsideOptionsRoundTrips pins the
// edit-config safety contract: a valid config value the select's Options
// don't offer (validators accept 1..100 masters) must survive
// LoadFromConfig -> Apply byte-identical instead of being coerced to the
// cursor's default.
func TestDataDrivenStep_LoadFromConfig_SelectOutsideOptionsRoundTrips(t *testing.T) {
	def := &StepDefinition{
		ID: StepIDBasics,
		Sections: []SectionDefinition{{
			Fields: []FieldDefinition{{
				Key:       "count",
				Label:     "control plane nodes",
				Default:   "3",
				Type:      FieldTypeSelect,
				Options:   []string{"1", "3", "5"},
				ConfigSet: SetInt(func(c *config.Config, v int) { c.Topology.ControlPlane.Count = v }),
				ConfigGet: GetInt(func(c *config.Config) int { return c.Topology.ControlPlane.Count }),
			}},
		}},
	}
	step := NewDataDrivenStep(def)
	cfg := &config.Config{}
	cfg.Topology.ControlPlane.Count = 7

	step.LoadFromConfig(cfg, true)

	out := &config.Config{}
	if err := step.Apply(out); err != nil {
		t.Fatalf("Apply(): %v", err)
	}
	if out.Topology.ControlPlane.Count != 7 {
		t.Fatalf("Apply() wrote count %d, want 7 preserved", out.Topology.ControlPlane.Count)
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

func TestDataDrivenDraftCursorRestoresSafeFieldAndOmitsCredentialField(t *testing.T) {
	def := testStepDefinition()
	def.Sections[0].Fields = append(def.Sections[0].Fields,
		FieldDefinition{Key: "api_token", Label: "API token", Type: FieldTypePassword},
	)
	step := NewDataDrivenStep(def)
	_ = step.form.Focus()
	step.form.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	if got := step.DraftFieldKey(); got != "count" {
		t.Fatalf("DraftFieldKey() = %q, want count", got)
	}
	if !step.SetDraftFieldKey("name") {
		t.Fatal("SetDraftFieldKey(name) = false")
	}
	step.SetFocused(true)
	_ = step.Init()
	if got := step.DraftFieldKey(); got != "name" {
		t.Errorf("restored DraftFieldKey() = %q, want name", got)
	}
	if step.SetDraftFieldKey("api_token") {
		t.Fatal("SetDraftFieldKey(api_token) = true, want credential fields rejected")
	}
}

// pairedTestDefinition declares two short text fields sharing a PairKey, in
// a section of their own so the surrounding section head never interferes
// with the row-join assertions below.
func pairedTestDefinition() *StepDefinition {
	return &StepDefinition{
		ID:    StepIDBasics,
		Title: "paired test step",
		Sections: []SectionDefinition{
			{
				Title: "pair section",
				Fields: []FieldDefinition{
					{Key: "field_a", Label: "field a", PairKey: "p"},
					{Key: "field_b", Label: "field b", PairKey: "p"},
				},
			},
		},
	}
}

// TestDataDrivenStep_PairedFieldsRenderOnOneRow pins mechanism 2: two
// fields sharing a PairKey join into a single visual row once the form
// column is wide enough, with one combined LineSpan covering both.
func TestDataDrivenStep_PairedFieldsRenderOnOneRow(t *testing.T) {
	step := NewDataDrivenStep(pairedTestDefinition())
	step.SetFocused(true)

	view := tuitest.StripANSI(step.View(100, 24))
	found := false
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "field a") && strings.Contains(line, "field b") {
			found = true
		}
	}
	if !found {
		t.Fatalf("View(100) = %q, want a row containing both paired labels", view)
	}

	if cmd := step.form.FocusField(0, 0); cmd != nil {
		cmd()
	}
	spanA, okA := step.FocusedSpan()
	if cmd := step.form.FocusField(0, 1); cmd != nil {
		cmd()
	}
	spanB, okB := step.FocusedSpan()
	if !okA || !okB || spanA != spanB {
		t.Fatalf("FocusedSpan() for paired fields = %v (ok=%v), %v (ok=%v), want identical spans", spanA, okA, spanB, okB)
	}
}

// TestDataDrivenStep_PairedFieldsFallBackBelowCompactFloor pins the
// pairMinInnerWidth floor: below the 80-column "ordinary" tier, a declared
// pair renders single column instead of joining a row.
func TestDataDrivenStep_PairedFieldsFallBackBelowCompactFloor(t *testing.T) {
	step := NewDataDrivenStep(pairedTestDefinition())
	step.SetFocused(true)

	view := tuitest.StripANSI(step.View(60, 24))
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "field a") && strings.Contains(line, "field b") {
			t.Fatalf("View(60) = %q, want field a and field b on separate rows below the pairing floor", view)
		}
	}
}

// TestDataDrivenStep_PairedFieldsReserveDefaultTagSymmetrically pins the
// controller ruling that a pair mixing a default-tagged field with an
// untagged one reserves the tag's room in both halves, so the two boxes
// stay the same width rather than the tagged one rendering visibly
// narrower than its sibling.
func TestDataDrivenStep_PairedFieldsReserveDefaultTagSymmetrically(t *testing.T) {
	def := &StepDefinition{
		ID:    StepIDBasics,
		Title: "mixed default pair test",
		Sections: []SectionDefinition{
			{
				Title: "pair section",
				Fields: []FieldDefinition{
					{Key: "has_default", Label: "has default", Default: "x", PairKey: "p"},
					{Key: "no_default", Label: "no default", PairKey: "p"},
				},
			},
		},
	}
	step := NewDataDrivenStep(def)
	step.SetFocused(true)

	view := tuitest.StripANSI(step.View(100, 24))
	lines := strings.Split(view, "\n")
	var boxTop string
	for _, line := range lines {
		if strings.Contains(line, "╭") {
			boxTop = line
			break
		}
	}
	if boxTop == "" {
		t.Fatalf("View(100) = %q, want a box border row", view)
	}
	openA := strings.Index(boxTop, "╭")
	closeA := strings.Index(boxTop[openA:], "╮") + openA
	openB := strings.Index(boxTop[closeA:], "╭") + closeA
	closeB := strings.Index(boxTop[openB:], "╮") + openB
	if openA < 0 || closeA < openA || openB < closeA || closeB < openB {
		t.Fatalf("box border row = %q, want two boxes", boxTop)
	}
	leftWidth := closeA - openA + 1
	rightWidth := closeB - openB + 1
	if leftWidth != rightWidth {
		t.Errorf("paired box widths = %d (default-tagged) vs %d (untagged), want equal", leftWidth, rightWidth)
	}
}

// absenceNoteTestDefinition declares a toggle field and a section hidden
// until the toggle flips to "yes", carrying an AbsenceNote.
func absenceNoteTestDefinition() *StepDefinition {
	return &StepDefinition{
		ID:    StepIDBasics,
		Title: "absence note test step",
		Sections: []SectionDefinition{
			{
				Title: "toggle",
				Fields: []FieldDefinition{
					{Key: "enabled", Label: "enabled", Default: "no"},
				},
			},
			{
				Title: "conditional",
				Visible: func(values map[string]string) bool {
					return values["enabled"] == testValYes
				},
				AbsenceNote: func(values map[string]string) string {
					if values["enabled"] == testValYes {
						return ""
					}
					return "conditional settings appear when enabled."
				},
				Fields: []FieldDefinition{
					{Key: "detail", Label: "detail"},
				},
			},
		},
	}
}

// TestDataDrivenStep_AbsenceNoteRendersWhenSectionHidden pins mechanism 4:
// a hidden conditional section renders its AbsenceNote line instead of
// vanishing with zero output.
func TestDataDrivenStep_AbsenceNoteRendersWhenSectionHidden(t *testing.T) {
	step := NewDataDrivenStep(absenceNoteTestDefinition())
	step.SetFocused(true)

	view := tuitest.StripANSI(step.View(100, 24))
	if !strings.Contains(view, "conditional settings appear when enabled.") {
		t.Fatalf("View() = %q, want the hidden section's AbsenceNote visible", view)
	}
	if strings.Contains(view, "detail") {
		t.Fatalf("View() = %q, want the hidden section's own field to stay hidden", view)
	}
}

// TestDataDrivenStep_AbsenceNoteClearsWhenSectionBecomesVisible pins the
// other half: once the section is visible, its real content renders and
// the absence note disappears.
func TestDataDrivenStep_AbsenceNoteClearsWhenSectionBecomesVisible(t *testing.T) {
	step := NewDataDrivenStep(absenceNoteTestDefinition())
	step.setValue("enabled", testValYes)
	step.SetFocused(true)

	view := tuitest.StripANSI(step.View(100, 24))
	if strings.Contains(view, "appear when enabled") {
		t.Fatalf("View() = %q, want no AbsenceNote once the section is visible", view)
	}
	if !strings.Contains(view, "detail") {
		t.Fatalf("View() = %q, want the now-visible section's field rendered", view)
	}
}

// collapsibleTestDefinition declares a primary section plus a Collapsible
// "advanced" section with one Required field — the shape the HARD
// CONSTRAINT exists to protect (a fold must never hide required work).
func collapsibleTestDefinition() *StepDefinition {
	return &StepDefinition{
		ID:    StepIDBasics,
		Title: "collapsible test step",
		Sections: []SectionDefinition{
			{
				Title: "primary",
				Fields: []FieldDefinition{
					{Key: "name", Label: "name"},
				},
			},
			{
				Title:       "advanced",
				Collapsible: true,
				FoldSummary: func(map[string]string) []tui.FactRow {
					return []tui.FactRow{{Key: "status", Value: "ok"}}
				},
				Fields: []FieldDefinition{
					{Key: "hidden_required", Label: "hidden required", Required: true},
				},
			},
		},
	}
}

// TestDataDrivenStep_CollapsibleSectionCollapsesWhenCompleteAndUnfocused
// pins the fold's quiet state: complete, unfocused, it renders one summary
// line instead of its field.
func TestDataDrivenStep_CollapsibleSectionCollapsesWhenCompleteAndUnfocused(t *testing.T) {
	step := NewDataDrivenStep(collapsibleTestDefinition())
	step.setValue("hidden_required", "x")
	step.SetFocused(true)

	view := tuitest.StripANSI(step.View(100, 24))
	if strings.Contains(view, "hidden required") {
		t.Fatalf("View() = %q, want the collapsed fold to hide its field", view)
	}
	if !strings.Contains(view, "advanced") || !strings.Contains(view, "status") {
		t.Fatalf("View() = %q, want the fold's label and FoldSummary fact visible", view)
	}
}

// TestDataDrivenStep_CollapsibleSectionExpandsWhenFocusIsInside pins the
// focus-forced expansion: tabbing into the section renders it in full even
// though the user never pressed enter on the toggle.
func TestDataDrivenStep_CollapsibleSectionExpandsWhenFocusIsInside(t *testing.T) {
	step := NewDataDrivenStep(collapsibleTestDefinition())
	step.setValue("hidden_required", "x")
	step.SetFocused(true)

	if cmd := step.form.FocusField(1, 0); cmd != nil {
		cmd()
	}
	view := tuitest.StripANSI(step.View(100, 24))
	if !strings.Contains(view, "hidden required") {
		t.Fatalf("View() = %q, want the fold's field visible once focus is inside its section", view)
	}
}

// TestDataDrivenStep_CollapsibleSectionAutoExpandsWhenRequiredFieldInvalid
// pins the HARD CONSTRAINT itself: a collapsed fold containing a
// required-and-empty field must auto-expand even while focus sits
// elsewhere, so the error is never silently hidden.
func TestDataDrivenStep_CollapsibleSectionAutoExpandsWhenRequiredFieldInvalid(t *testing.T) {
	step := NewDataDrivenStep(collapsibleTestDefinition())
	step.SetFocused(true) // focuses "name"; hidden_required stays empty

	step.form.TouchAll()

	view := tuitest.StripANSI(step.View(100, 24))
	if !strings.Contains(view, "hidden required") {
		t.Fatalf("View() = %q, want a required-and-invalid field to force its collapsed fold open", view)
	}
}

// TestDataDrivenStep_CollapsibleSectionValidateReachesHiddenField pins that
// Validate/TouchAll/FocusFirstInvalid treat a Collapsible section exactly
// like a visible one: enter's forced submission attempt focuses the hidden
// required field rather than silently failing.
func TestDataDrivenStep_CollapsibleSectionValidateReachesHiddenField(t *testing.T) {
	step := NewDataDrivenStep(collapsibleTestDefinition())
	step.SetFocused(true)

	_, cmd := step.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Update(enter) on an invalid collapsed fold: want a cmd, got nil")
	}
	if !containsFocusChanged(cmd) {
		t.Fatal("Update(enter) on an invalid collapsed fold did not emit FocusChangedMsg")
	}
	if got := step.form.FocusedField(); got != step.getField("hidden_required") {
		t.Fatalf("FocusedField() after enter = %v, want the hidden required field", got)
	}
}

// TestDataDrivenStep_CollapsibleSectionEnterStickyExpand pins mechanism 3's
// enter binding: toggling the fold open via enter keeps it open even after
// focus moves elsewhere, since the latch is sticky rather than
// focus-derived.
func TestDataDrivenStep_CollapsibleSectionEnterStickyExpand(t *testing.T) {
	step := NewDataDrivenStep(collapsibleTestDefinition())
	step.setValue("hidden_required", "x")
	step.SetFocused(true)

	if cmd := step.form.FocusField(1, 0); cmd != nil {
		cmd()
	}
	if _, cmd := step.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		cmd()
	}
	if cmd := step.form.FocusField(0, 0); cmd != nil {
		cmd()
	}

	view := tuitest.StripANSI(step.View(100, 24))
	if !strings.Contains(view, "hidden required") {
		t.Fatalf("View() = %q, want the fold to stay expanded via its sticky latch after focus left", view)
	}
}

func TestTableCellCommitDoesNotCompleteStep(t *testing.T) {
	step := NewDataDrivenStep(&StepDefinition{ID: StepIDAdvanced, Sections: []SectionDefinition{{Title: "labels", Fields: []FieldDefinition{{Key: "labels", Label: "labels", Type: FieldTypeKeyValue}}}}})
	step.SetFocused(true)
	step.Init()
	step.Update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	if !step.OwnsKey(tea.KeyPressMsg{Code: tea.KeyEnter}) {
		t.Fatal("editor does not own enter")
	}
	step.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	step.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if step.Value("labels") != "" {
		t.Fatal("cancel retained edited cell")
	}
	step.Update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	_, cmd := step.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		if _, complete := cmd().(StepCompleteMsg); complete {
			t.Fatal("cell commit advanced step")
		}
	}
	if step.OwnsKey(tea.KeyPressMsg{Code: tea.KeyEnter}) {
		t.Fatal("cell remained in edit mode")
	}
}
