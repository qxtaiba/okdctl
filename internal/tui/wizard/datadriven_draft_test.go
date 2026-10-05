package wizard

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
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
