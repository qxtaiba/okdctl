package distribution_test

import (
	"context"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/distribution"
)

func TestBuildSteps_PanicsOnMissingIdentity(t *testing.T) {
	t.Parallel()
	exec := func(_ context.Context) error { return nil }
	cases := []struct {
		name string
		def  distribution.StepDef
		want string
	}{
		{"empty id", distribution.StepDef{Name: "test step", Exec: exec}, "empty ID"},
		{"empty name", distribution.StepDef{ID: "test-step", Exec: exec}, "test-step has empty Name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				msg, ok := recover().(string)
				if !ok || !strings.Contains(msg, tc.want) {
					t.Fatalf("panic = %q, want one containing %q", msg, tc.want)
				}
			}()
			distribution.BuildSteps([]distribution.StepDef{tc.def})
		})
	}
}

func TestBuildSteps_AcceptsStepWithoutAlreadyDone(t *testing.T) {
	t.Parallel()
	defs := []distribution.StepDef{{ID: "a", Name: "a", Exec: func(_ context.Context) error { return nil }}}
	if got := distribution.BuildSteps(defs); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("BuildSteps = %+v, want the single step back", got)
	}
}
