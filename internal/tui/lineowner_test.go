package tui

import "testing"

// hasOwner is test-only; production code never queries ownership.
func (r *lineRegistry) hasOwner() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.owner != nil
}

// noopOwner carries a name so distinct instances are distinct pointers
// (zero-size structs can alias).
type noopOwner struct{ name string }

func (*noopOwner) clearLine() {}

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
