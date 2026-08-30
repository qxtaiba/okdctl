package addon

import (
	"context"
	"slices"
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

func TestResolve_Order(t *testing.T) {
	cases := []struct {
		name   string
		addons []Addon
		want   []string
	}{
		{"no deps sorts by name", []Addon{fa("zebra", 0), fa("apple", 0), fa("mango", 0)}, []string{"apple", "mango", "zebra"}},
		{"priority breaks ties", []Addon{fa("b", 2), fa("a", 1), fa("c", 3)}, []string{"a", "b", "c"}},
		{"chain orders leaf first", []Addon{fa("a", 0, "b"), fa("b", 0, "c"), fa("c", 0)}, []string{"c", "b", "a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.addons)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(names(got), tc.want) {
				t.Errorf("Resolve order = %v, want %v", names(got), tc.want)
			}
		})
	}
}

func TestResolve_Errors(t *testing.T) {
	cases := []struct {
		name        string
		addons      []Addon
		wantSubstrs []string
	}{
		{"missing dep", []Addon{fa("a", 0, "missing")}, []string{"depends on", "a", "missing"}},
		{"circular", []Addon{fa("a", 0, "b"), fa("b", 0, "a")}, []string{"circular dependency detected"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Resolve(tc.addons)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			for _, want := range tc.wantSubstrs {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err.Error(), want)
				}
			}
		})
	}
}
