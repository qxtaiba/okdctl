package addon

import (
	"context"
	"strings"
	"testing"
)

type fakeAddon struct {
	name     string
	deps     []string
	priority int
}

func (f *fakeAddon) Info() Metadata {
	return Metadata{Name: f.name, Dependencies: f.deps, Priority: f.priority}
}

func (f *fakeAddon) Install(_ context.Context, _ *Environment) error   { return nil }
func (f *fakeAddon) Verify(_ context.Context, _ *Environment) error    { return nil }
func (f *fakeAddon) Uninstall(_ context.Context, _ *Environment) error { return nil }

func fa(name string, priority int, deps ...string) *fakeAddon {
	return &fakeAddon{name: name, priority: priority, deps: deps}
}

func names(addons []Addon) []string {
	out := make([]string, len(addons))
	for i, a := range addons {
		out[i] = a.Info().Name
	}
	return out
}

func TestResolve_NoDeps_SortsByName(t *testing.T) {
	got, err := Resolve([]Addon{fa("zebra", 0), fa("apple", 0), fa("mango", 0)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"apple", "mango", "zebra"}
	for i, n := range want {
		if names(got)[i] != n {
			t.Errorf("position %d: got %q, want %q", i, names(got)[i], n)
		}
	}
}

func TestResolve_Priority_BreaksTies(t *testing.T) {
	got, err := Resolve([]Addon{fa("b", 2), fa("a", 1), fa("c", 3)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a", "b", "c"}
	for i, n := range want {
		if names(got)[i] != n {
			t.Errorf("position %d: got %q, want %q", i, names(got)[i], n)
		}
	}
}

func TestResolve_Chain_OrdersLeafFirst(t *testing.T) {
	a := fa("a", 0, "b")
	b := fa("b", 0, "c")
	c := fa("c", 0)
	got, err := Resolve([]Addon{a, b, c})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"c", "b", "a"}
	for i, n := range want {
		if names(got)[i] != n {
			t.Errorf("position %d: got %q, want %q", i, names(got)[i], n)
		}
	}
}

func TestResolve_MissingDep_ReturnsError(t *testing.T) {
	_, err := Resolve([]Addon{fa("a", 0, "missing")})
	if err == nil {
		t.Fatal("expected error for missing dependency, got nil")
	}
	if !strings.Contains(err.Error(), "depends on") {
		t.Errorf("error %q does not contain \"depends on\"", err.Error())
	}
	if !strings.Contains(err.Error(), "a") {
		t.Errorf("error %q does not mention addon name \"a\"", err.Error())
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error %q does not mention missing dep name \"missing\"", err.Error())
	}
}

func TestResolve_Circular_ReturnsError(t *testing.T) {
	a := fa("a", 0, "b")
	b := fa("b", 0, "a")
	_, err := Resolve([]Addon{a, b})
	if err == nil {
		t.Fatal("expected error for circular dependency, got nil")
	}
	if !strings.Contains(err.Error(), "circular dependency detected") {
		t.Errorf("error %q does not contain \"circular dependency detected\"", err.Error())
	}
}
