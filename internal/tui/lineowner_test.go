package tui

import "testing"

// hasOwner reports whether a line owner is currently registered. Test-only
// helper: production code never queries ownership, so this lives beside the
// tests rather than in lineowner.go.
func (r *lineRegistry) hasOwner() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.owner != nil
}

// noopOwner carries a name so distinct instances are distinct pointers — a
// pointer to a zero-size struct may compare equal to another, which would
// defeat the owner-identity guard under test.
type noopOwner struct{ name string }

func (*noopOwner) clearLine() {}

// TestLineRegistry_PaintGuardsOnOwner proves paint runs the repaint only for
// the current owner: after a second owner registers (the spinner a step body
// spawns while the checklist is registered), the displaced owner's paint is a
// no-op so it cannot overwrite the active owner's line.
func TestLineRegistry_PaintGuardsOnOwner(t *testing.T) {
	var reg lineRegistry
	a := &noopOwner{name: "a"}
	b := &noopOwner{name: "b"}

	reg.register(a)
	var aPainted bool
	reg.paint(a, func() { aPainted = true })
	if !aPainted {
		t.Fatal("current owner a was not allowed to paint")
	}

	reg.register(b)
	var stalePainted bool
	reg.paint(a, func() { stalePainted = true })
	if stalePainted {
		t.Fatal("displaced owner a painted over the current owner b")
	}
	var bPainted bool
	reg.paint(b, func() { bPainted = true })
	if !bPainted {
		t.Fatal("current owner b was not allowed to paint")
	}

	reg.release(b)
	var afterRelease bool
	reg.paint(b, func() { afterRelease = true })
	if afterRelease {
		t.Fatal("released owner still painted")
	}
}
