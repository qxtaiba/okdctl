package components

import (
	"strings"
	"testing"
)

func TestInputGroup_ViewMatchesFieldViews(t *testing.T) {
	g := NewInputGroup(
		NewInputField("name", "cluster"),
		NewPasswordField("token", "secret"),
		NewSelectField("approve", []string{"no", "yes"}),
	)
	g.SetWidth(60)

	views := g.FieldViews()
	if len(views) != len(g.Fields()) {
		t.Fatalf("FieldViews() returned %d views, want %d", len(views), len(g.Fields()))
	}
	if got, want := g.View(), strings.Join(views, "\n\n"); got != want {
		t.Fatalf("View() does not equal FieldViews() joined by a blank row:\n got %q\nwant %q", got, want)
	}
}

func TestInputGroup_FieldViewsEmptyGroup(t *testing.T) {
	g := NewInputGroup()
	if views := g.FieldViews(); len(views) != 0 {
		t.Fatalf("FieldViews() = %v, want empty", views)
	}
	if got := g.View(); got != "" {
		t.Fatalf("View() = %q, want empty", got)
	}
}
