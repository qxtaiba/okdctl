package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
	"github.com/qxtaiba/okdctl/internal/wizarddraft"
)

func loadWizardDraft(path string) (*wizarddraft.Draft, bool) {
	store := wizarddraft.New(path)
	draft, err := store.Load()
	if err != nil {
		logutil.Warn("ignoring invalid wizard draft", logutil.LF("err", err))
		return nil, false
	}
	if draft == nil {
		return nil, false
	}

	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return draft, true
	}
	if err != nil {
		logutil.Warn("ignoring wizard draft because config timestamp is unavailable", logutil.LF("err", err))
		return nil, false
	}
	if !draft.UpdatedAt.After(info.ModTime()) {
		return nil, false
	}
	return draft, true
}

func draftResumeLabel(draft *wizarddraft.Draft, now time.Time) string {
	step := strings.ReplaceAll(string(draft.Cursor.StepID), "-", " ")
	return fmt.Sprintf("resume draft · at %s · edited %s", step, formatDraftAge(now, draft.UpdatedAt))
}

func formatDraftAge(now, updatedAt time.Time) string {
	age := now.Sub(updatedAt)
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return fmt.Sprintf("%dm ago", int(age/time.Minute))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(age/time.Hour))
	default:
		return fmt.Sprintf("%dd ago", int(age/(24*time.Hour)))
	}
}

func configureDraftResume(built wizard.BuiltSteps, draft *wizarddraft.Draft, now time.Time) {
	for _, step := range built.Steps {
		if hub, ok := step.(*steps.WelcomeStep); ok {
			hub.SetDraftResume(draft.Cursor.StepID, draft.Cursor.FieldKey, draftResumeLabel(draft, now))
			return
		}
	}
}

func wizardDraftPath(path string) string {
	if path == "" {
		return "okdctl.yaml"
	}
	return path
}

func clearWizardDraft(path string) error {
	if err := wizarddraft.New(wizardDraftPath(path)).Clear(); err != nil {
		return fmt.Errorf("clear wizard draft: %w", err)
	}
	return nil
}

func clearWizardDraftLocked(projectRoot, path string) error {
	return withProjectLock(projectRoot, "clear wizard draft", func() error {
		return clearWizardDraft(path)
	})
}
