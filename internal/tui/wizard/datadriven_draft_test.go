package wizard

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

func TestDataDrivenInputEditRequestsDraftSync(t *testing.T) {
	step := NewDataDrivenStep(&StepDefinition{
		ID:    StepIDBasics,
		Title: "basics",
		Sections: []SectionDefinition{{Fields: []FieldDefinition{{
			Key:       "cluster_name",
			Type:      FieldTypeText,
			ConfigSet: func(cfg *config.Config, value string) error { cfg.Cluster.Name = value; return nil },
		}}}},
	})
	step.Init()
	_, cmd := step.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd == nil {
		t.Fatal("input edit returned no config sync command")
	}
	msg := cmd()
	if sync, ok := msg.(ConfigSyncMsg); ok && sync.StepID == StepIDBasics {
		return
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			if sync, ok := child().(ConfigSyncMsg); ok && sync.StepID == StepIDBasics {
				return
			}
		}
	}
	t.Fatalf("edit command returned %T(%+v), want basics ConfigSyncMsg", msg, msg)
}

func TestDataDrivenFieldHistoryRestoresSafeValuesAndSkipsPasswords(t *testing.T) {
	step := NewDataDrivenStep(&StepDefinition{
		ID:    StepIDProxmox,
		Title: "proxmox",
		Sections: []SectionDefinition{{Fields: []FieldDefinition{
			{Key: "host", Type: FieldTypeText},
			{Key: "auth", Type: FieldTypePassword},
		}}},
	})
	step.SetFieldHistory(map[string][]string{
		"proxmox/host": {"pve.example.test"},
		"proxmox/auth": {"password-sentinel"},
	})
	step.Init()
	step.getField("host").(*components.InputField).Focus()
	step.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	step.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := step.Value("host"); got != "pve.example.test" {
		t.Fatalf("restored host = %q", got)
	}
	password := step.getField("auth").(*components.InputField)
	password.Focus()
	password.SetValue("password-sentinel")
	password.Blur()
	if got := step.FieldHistory(); len(got) != 1 {
		t.Fatalf("password entered history: %v", got)
	}
}
