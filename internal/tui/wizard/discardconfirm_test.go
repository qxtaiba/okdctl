package wizard

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

var ctrlC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}

type keyRecordingStep struct {
	fakeStep
	keys []tea.KeyPressMsg
}

func (s *keyRecordingStep) Update(msg tea.Msg) (WizardStep, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		s.keys = append(s.keys, k)
	}
	return s, nil
}

func configureModel(t *testing.T, id StepID) (*Model, *keyRecordingStep) {
	t.Helper()
	step := &keyRecordingStep{fakeStep: fakeStep{id: id}}
	m := NewModel([]WizardStep{step}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)
	return m, step
}

func editedConfigureModel(t *testing.T) (*Model, *keyRecordingStep) {
	t.Helper()
	m, step := configureModel(t, StepIDBasics)
	m.Config().Cluster.Name = "edited"
	return m, step
}

func showsDiscardPrompt(m *Model) bool {
	return strings.Contains(tuitest.StripANSI(m.View().Content), discardPrompt)
}

func assertCancelled(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if !m.quitting {
		t.Fatal("wizard did not quit")
	}
	if got := m.Result().Outcome; got != OutcomeCancelled {
		t.Errorf("Outcome = %v, want cancelled", got)
	}
	if cmd == nil {
		t.Fatal("quit returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("quit command produced %T, want tea.QuitMsg", cmd())
	}
}

func TestCtrlCOnEditedConfigureStepAsksBeforeDiscarding(t *testing.T) {
	m, _ := editedConfigureModel(t)
	if showsDiscardPrompt(m) {
		t.Fatal("discard prompt shown before ctrl+c")
	}

	m = update(t, m, ctrlC)

	if m.quitting {
		t.Fatal("first ctrl+c quit with unsaved edits")
	}
	if !showsDiscardPrompt(m) {
		t.Errorf("first ctrl+c did not show the discard prompt:\n%s", tuitest.StripANSI(m.View().Content))
	}
}

func TestDiscardPromptFitsTheStatusRow(t *testing.T) {
	for _, tc := range []struct {
		w, h int
		want string
	}{
		{60, 20, "discard unsaved changes?"},
		{80, 24, discardPrompt},
	} {
		m, _ := editedConfigureModel(t)
		tuitest.RenderAt(t, m, tc.w, tc.h)

		m = update(t, m, ctrlC)

		frame := m.View().Content
		tuitest.AssertFits(t, frame, tc.w, tc.h)
		if !strings.Contains(tuitest.StripANSI(frame), tc.want) {
			t.Errorf("%dx%d frame is missing %q:\n%s", tc.w, tc.h, tc.want, tuitest.StripANSI(frame))
		}
	}
}

func TestSecondCtrlCDiscardsUnsavedEdits(t *testing.T) {
	m, _ := editedConfigureModel(t)
	m = update(t, m, ctrlC)

	mm, cmd := m.Update(ctrlC)

	assertCancelled(t, mm.(*Model), cmd)
}

func TestConfirmKeyDiscardsUnsavedEdits(t *testing.T) {
	m, step := editedConfigureModel(t)
	m = update(t, m, ctrlC)

	mm, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})

	assertCancelled(t, mm.(*Model), cmd)
	if len(step.keys) != 0 {
		t.Errorf("confirm key reached the step: %v", step.keys)
	}
}

func TestOtherKeyReturnsToTheFormAndReArmsTheQuestion(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{
		{Code: 'n', Text: "n"},
		{Code: tea.KeyEscape},
		{Code: tea.KeyEnter},
	} {
		t.Run(k.String(), func(t *testing.T) {
			m, step := editedConfigureModel(t)
			m = update(t, m, ctrlC)

			m = update(t, m, k)

			if m.quitting {
				t.Fatal("a key other than ctrl+c or y quit the wizard")
			}
			if showsDiscardPrompt(m) {
				t.Error("discard prompt still shown after returning to the form")
			}
			if len(step.keys) != 0 {
				t.Errorf("the dismissing key reached the step: %v", step.keys)
			}

			m = update(t, m, ctrlC)
			if m.quitting || !showsDiscardPrompt(m) {
				t.Error("a later ctrl+c must ask again, not quit")
			}
		})
	}
}

func TestCtrlCWithoutEditsQuitsImmediately(t *testing.T) {
	m, _ := configureModel(t, StepIDBasics)

	mm, cmd := m.Update(ctrlC)

	assertCancelled(t, mm.(*Model), cmd)
}

func TestCtrlCOffTheConfigureFlowQuitsImmediately(t *testing.T) {
	for _, id := range []StepID{StepIDWelcome, "op"} {
		t.Run(string(id), func(t *testing.T) {
			m, _ := configureModel(t, id)
			m.Config().Cluster.Name = "edited"

			mm, cmd := m.Update(ctrlC)

			assertCancelled(t, mm.(*Model), cmd)
		})
	}
}

func TestCtrlCAsksOnEveryConfigureStep(t *testing.T) {
	for _, id := range []StepID{
		StepIDDistribution, StepIDProxmox, StepIDBasics, StepIDNodePlacement, StepIDNetworking,
		StepIDResources, StepIDAddons, StepIDFiles, StepIDAdvanced, StepIDReview,
	} {
		t.Run(string(id), func(t *testing.T) {
			m, _ := configureModel(t, id)
			m.Config().Cluster.Name = "edited"

			m = update(t, m, ctrlC)

			if m.quitting || !showsDiscardPrompt(m) {
				t.Error("ctrl+c with unsaved edits must ask before quitting")
			}
		})
	}
}

func TestCredentialEditCountsAsUnsaved(t *testing.T) {
	edits := map[string]func(*config.ProxmoxConfig){
		"username":  func(p *config.ProxmoxConfig) { p.Username = "root@pam" },
		"password":  func(p *config.ProxmoxConfig) { p.Password.Set("hunter2") },
		"api token": func(p *config.ProxmoxConfig) { p.APIToken.Set("token-value") },
	}
	for name, edit := range edits {
		t.Run(name, func(t *testing.T) {
			m, _ := configureModel(t, StepIDProxmox)
			if m.Config().Provider.Proxmox == nil {
				t.Fatal("default config carries no proxmox provider block")
			}
			edit(m.Config().Provider.Proxmox)

			m = update(t, m, ctrlC)

			if m.quitting || !showsDiscardPrompt(m) {
				t.Error("ctrl+c after a credential edit must ask before quitting")
			}
		})
	}
}

func TestRevertedEditQuitsImmediately(t *testing.T) {
	m, _ := configureModel(t, StepIDBasics)
	original := m.Config().Cluster.Name
	m.Config().Cluster.Name = "edited"
	m.Config().Cluster.Name = original

	mm, cmd := m.Update(ctrlC)

	assertCancelled(t, mm.(*Model), cmd)
}

func TestTypingInAFormFieldArmsTheDiscardQuestion(t *testing.T) {
	step := NewDataDrivenStep(&StepDefinition{
		ID:    StepIDBasics,
		Title: "basics",
		Sections: []SectionDefinition{{Fields: []FieldDefinition{{
			Key:       "cluster_name",
			Label:     "cluster name",
			Type:      FieldTypeText,
			ConfigSet: SetString(func(cfg *config.Config, v string) { cfg.Cluster.Name = v }),
			ConfigGet: GetString(func(cfg *config.Config) string { return cfg.Cluster.Name }),
		}}}},
	})
	cfg := config.DefaultConfig()
	step.LoadFromConfig(cfg, true)
	m := NewModel([]WizardStep{step}, cfg)
	tuitest.RenderAt(t, m, 100, 30)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	deliver(m, cmd)

	m = update(t, m, ctrlC)
	if m.quitting || !showsDiscardPrompt(m) {
		t.Errorf("ctrl+c after typing into a field must ask before quitting (cluster name = %q)", cfg.Cluster.Name)
	}
}

func TestQuitGuardStillOwnsCtrlCAheadOfTheDiscardQuestion(t *testing.T) {
	g := &guardedStep{nopStep: nopStep{BaseStep: NewBaseStep(StepIDBasics, "basics", "")}, intercepts: true}
	m := NewModel([]WizardStep{g}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)
	m.Config().Cluster.Name = "edited"

	m = update(t, m, ctrlC)

	if g.calls != 1 {
		t.Fatalf("InterceptQuit calls = %d, want 1", g.calls)
	}
	if m.quitting || showsDiscardPrompt(m) {
		t.Error("an intercepting quit guard must consume ctrl+c before the discard question")
	}
}

func deliver(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			deliver(m, c)
		}
		return
	}
	m.Update(msg)
}

func TestShutdownClearsTheSavedCredentials(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Password.Set("hunter2")
	cfg.Provider.Proxmox.APIToken.Set("token-value")
	m := NewModel([]WizardStep{&fakeStep{id: StepIDBasics}}, cfg)
	password, apiToken := m.saved.password, m.saved.apiToken
	if string(password) != "hunter2" || string(apiToken) != "token-value" {
		t.Fatalf("saved credentials = %q, %q; want the ones the flow opened with", password, apiToken)
	}

	m.shutdown()

	for _, b := range append(password, apiToken...) {
		if b != 0 {
			t.Fatal("shutdown left a saved credential byte in memory")
		}
	}
}

func TestSavedCredentialsDoNotAliasTheLiveConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Password.Set("hunter2")
	m := NewModel([]WizardStep{&fakeStep{id: StepIDBasics}}, cfg)

	live := cfg.Provider.Proxmox.Password.Bytes()
	live[0] = 'X'

	if string(m.saved.password) != "hunter2" {
		t.Errorf("saved password = %q; an edit to the live config reached it", m.saved.password)
	}
}
