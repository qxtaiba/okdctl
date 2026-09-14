package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func confirmDestroyFacts() []Fact {
	return []Fact{
		{Key: "cluster", Value: "grappleberry"},
		{Key: "domain", Value: "grappleberry.example.com"},
		{Key: "scope", Value: "full cluster (all VMs)"},
		{Key: "also removes", Value: "fcos iso, host files, firewall rules"},
	}
}

func TestConfirmBox(t *testing.T) {
	facts := confirmDestroyFacts()
	got := ConfirmBox("destroy", facts, IrreversibleWarning)

	for _, want := range []string{
		"confirm destroy", tui.IconWarning,
		"cluster", "grappleberry", "domain", "full cluster (all VMs)", "also removes",
		"irreversible — destroys the listed VM(s)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("confirm box missing %q:\n%s", want, got)
		}
	}

	for _, w := range []int{80, 120} {
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			tui.SetTerminalWidth(w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			out := ConfirmBox("destroy", facts, IrreversibleWarning)
			tuitest.AssertFits(t, out, w, 0)
			if w == 80 {
				tuitest.Golden(t, "confirm-destroy-80", out)
			}
		})
	}
}

func TestConfirmBoxReversibleHasNoRedLine(t *testing.T) {
	facts := []Fact{
		{Key: "cluster", Value: "grappleberry"},
		{Key: "domain", Value: "grappleberry.example.com"},
	}
	got := ConfirmBox("ingress update", facts, "")

	if strings.Contains(got, "irreversible") {
		t.Errorf("reversible confirm box must carry no irreversible line:\n%s", got)
	}
}
