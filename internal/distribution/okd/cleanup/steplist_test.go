package cleanup

import (
	"context"
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

func TestCleanupSteps_KindSelectsStepList(t *testing.T) {
	cases := []struct {
		kind Kind
		want []distribution.StepID
	}{
		{
			kind: Full,
			want: []distribution.StepID{
				StepCleanupWorkDir,
				StepCleanupWebServer,
				StepCleanupHAProxy,
				StepCleanupApache,
				StepCleanupDnsmasq,
				StepCleanupTerraform,
				StepCleanupPackages,
				StepCleanupIgnitionCerts,
				StepCleanupSummary,
			},
		},
		{kind: WorkOnly, want: []distribution.StepID{StepCleanupWorkDir, StepCleanupSummary}},
		{kind: WebOnly, want: []distribution.StepID{StepCleanupWebServer, StepCleanupSummary}},
		{kind: HAProxyOnly, want: []distribution.StepID{StepCleanupHAProxy, StepCleanupSummary}},
		{kind: TerraformOnly, want: []distribution.StepID{StepCleanupTerraform, StepCleanupSummary}},
	}

	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			opts := &Options{
				BaseOptions: phase.BaseOptions{WorkDir: t.TempDir(), ProjectRoot: t.TempDir()},
				Kind:        tc.kind,
			}
			defs := cleanupSteps(opts, logutil.NopLogger)
			if len(defs) != len(tc.want) {
				t.Fatalf("step count = %d; want %d", len(defs), len(tc.want))
			}
			for i, d := range defs {
				if d.ID != tc.want[i] {
					t.Errorf("step[%d] = %q; want %q", i, d.ID, tc.want[i])
				}
			}
		})
	}
}

func TestPhaseExecute_RejectsInvalidKind(t *testing.T) {
	p := New(phase.WithLogger(logutil.NopLogger))
	for _, kind := range []Kind{"", "bogus"} {
		opts := &Options{
			BaseOptions: phase.BaseOptions{WorkDir: t.TempDir()},
			Kind:        kind,
		}
		err := p.Execute(context.Background(), opts)
		var cfgErr *errtypes.ConfigError
		if !errors.As(err, &cfgErr) {
			t.Errorf("kind %q: err = %v; want *errtypes.ConfigError", kind, err)
		}
	}
}
