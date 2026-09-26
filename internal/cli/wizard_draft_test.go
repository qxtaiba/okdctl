package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/wizarddraft"
)

func TestLoadWizardDraftRequiresItToBeNewerThanConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "okdctl.yaml")
	store := wizarddraft.New(path)
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "draft"
	configTime := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	draftTime := configTime.Add(time.Hour)
	if err := os.WriteFile(path, []byte("config"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, configTime, configTime); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(cfg, wizarddraft.Cursor{StepID: wizard.StepIDNetworking}, draftTime); err != nil {
		t.Fatal(err)
	}

	got, active := loadWizardDraft(path)
	if !active || got == nil || got.Config.Cluster.Name != "draft" {
		t.Fatalf("newer draft = (%v, %t), want active draft", got, active)
	}

	if err := store.Save(cfg, wizarddraft.Cursor{StepID: wizard.StepIDNetworking}, configTime); err != nil {
		t.Fatal(err)
	}
	got, active = loadWizardDraft(path)
	if active || got != nil {
		t.Fatalf("stale draft = (%v, %t), want ignored", got, active)
	}
}

func TestLoadWizardDraftWithoutConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "okdctl.yaml")
	store := wizarddraft.New(path)
	if err := store.Save(config.DefaultConfig(), wizarddraft.Cursor{StepID: wizard.StepIDBasics}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got, active := loadWizardDraft(path); !active || got == nil {
		t.Fatalf("draft without config = (%v, %t), want active", got, active)
	}
}

func TestFormatDraftAge(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		age  time.Duration
		want string
	}{
		{age: 20 * time.Second, want: "just now"},
		{age: 5 * time.Minute, want: "5m ago"},
		{age: 2*time.Hour + 20*time.Minute, want: "2h ago"},
		{age: 3 * 24 * time.Hour, want: "3d ago"},
	} {
		if got := formatDraftAge(now, now.Add(-tc.age)); got != tc.want {
			t.Errorf("formatDraftAge(%s) = %q, want %q", tc.age, got, tc.want)
		}
	}
}

func TestClearWizardDraftRemovesSidecar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "okdctl.yaml")
	store := wizarddraft.New(path)
	if err := store.Save(config.DefaultConfig(), wizarddraft.Cursor{StepID: wizard.StepIDBasics}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := clearWizardDraft(path); err != nil {
		t.Fatalf("clearWizardDraft: %v", err)
	}
	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Errorf("draft remains after successful save/deploy cleanup: %v", err)
	}
	if err := clearWizardDraft(path); err != nil {
		t.Errorf("clearWizardDraft should be idempotent: %v", err)
	}
}
