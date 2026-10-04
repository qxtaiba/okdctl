package components

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestCompactPasswordRendering(t *testing.T) {
	f := NewPasswordField("password", "optional")
	f.SetWidth(60)
	f.Help = "Preserve CASE in help"
	f.SetValue("SensitiveValue")
	inactive := f.View()
	if strings.Contains(inactive, "SensitiveValue") || strings.Contains(inactive, f.Help) {
		t.Fatal("inactive field leaks secret or shows help")
	}
	f.Focus()
	active := f.View()
	if strings.Contains(active, "SensitiveValue") || !strings.Contains(active, f.Help) {
		t.Fatal("focused password or help incorrect")
	}
	if lipgloss.Height(inactive) >= lipgloss.Height(active) {
		t.Fatal("inactive field is not compact")
	}
	f.Validator = func(string) error { return errors.New("Cannot read /Mixed/CASE: SensitiveValue") }
	if err := f.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
	out := f.View()
	if strings.Contains(out, "SensitiveValue") || !strings.Contains(out, "/Mixed/CASE") {
		t.Fatal("error lost case or exposed secret")
	}
}

func TestCursorMotionDoesNotRepeatValidation(t *testing.T) {
	f := NewInputField("path", "")
	f.SetValue("/fixture")
	f.Focus()
	calls := 0
	f.Validator = func(string) error { calls++; return nil }
	f.Valid()
	f.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	f.Valid()
	if calls != 1 {
		t.Fatalf("cursor movement reran validator: %d", calls)
	}
	f.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	f.Valid()
	if calls != 2 {
		t.Fatalf("edit did not rerun validator: %d", calls)
	}
}
