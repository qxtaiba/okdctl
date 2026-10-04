package addon

import (
	"context"
	"errors"
	"slices"
	"testing"
)

type statefulAddon struct {
	stubAddon
	resources     map[string]bool
	rollbackOrder *[]string
	cancel        context.CancelFunc
}

func (a *statefulAddon) Install(context.Context, *Environment) error {
	a.resources[a.meta.Name] = true
	if a.cancel != nil {
		a.cancel()
	}
	return a.installErr
}

func (a *statefulAddon) PrepareRollback(context.Context, *Environment) (func(context.Context) error, error) {
	if a.resources[a.meta.Name] {
		return nil, nil
	}
	return func(ctx context.Context) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		delete(a.resources, a.meta.Name)
		*a.rollbackOrder = append(*a.rollbackOrder, a.meta.Name)
		return nil
	}, nil
}

func TestCompensationPreservesExistingAndRemovesPartialInstall(t *testing.T) {
	for _, cancelDuringInstall := range []bool{false, true} {
		t.Run(map[bool]string{false: "partial failure", true: "cancellation"}[cancelDuringInstall], func(t *testing.T) {
			installFakeOC(t)
			resources := map[string]bool{"existing": true}
			var order []string
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			makeAddon := func(name string, priority int, deps []string) *statefulAddon {
				return &statefulAddon{stubAddon: stubAddon{meta: Metadata{Name: name, Priority: priority, Dependencies: deps}}, resources: resources, rollbackOrder: &order}
			}
			existing := makeAddon("existing", 1, nil)
			added := makeAddon("added", 2, []string{"existing"})
			failed := makeAddon("failed", 3, []string{"added"})
			if cancelDuringInstall {
				failed.cancel = cancel
			} else {
				failed.installErr = errors.New("partial install")
			}
			registerStubs(t, existing, added, failed)
			if err := NewManager(enabledCfg("existing", "added", "failed")).InstallOne(ctx, "failed"); err == nil {
				t.Fatal("failure lost")
			}
			if len(resources) != 1 || !resources["existing"] {
				t.Fatalf("wrong final resources: %v", resources)
			}
			if !slices.Equal(order, []string{"failed", "added"}) {
				t.Fatalf("wrong rollback order: %v", order)
			}
		})
	}
}
