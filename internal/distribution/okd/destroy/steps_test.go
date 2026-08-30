package destroy

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

func newPhaseWithCapture(h *testutil.CaptureHandler) *Phase {
	return &Phase{
		BasePhase: phase.NewBasePhase(
			phase.WithExecutor(executor.New()),
			phase.WithLogger(slog.New(h)),
		),
	}
}

// attrValue returns the String() of rec's first attr with key, or "" when absent.
func attrValue(rec *slog.Record, key string) string {
	var val string
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			val = a.Value.String()
			return false
		}
		return true
	})
	return val
}

func minimalConfig() *config.Config {
	return &config.Config{
		Provider: config.ProviderConfig{Proxmox: nil},
	}
}

func minimalOpts() *Options {
	return &Options{
		SkipTerraform: true,
		SkipCleanup:   true,
		SkipFirewall:  true,
		KeepISOs:      true,
	}
}

func TestDestroySteps_SuccessPath(t *testing.T) {
	h := &testutil.CaptureHandler{}
	defs := newPhaseWithCapture(h).destroySteps(context.Background(), minimalConfig(), minimalOpts())

	if defs[4].ID != StepPrintSummary {
		t.Fatalf("defs[4].ID = %q; want %q", defs[4].ID, StepPrintSummary)
	}
	if err := defs[4].Exec(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rec, ok := h.Last()
	if !ok {
		t.Fatal("no log records captured")
	}
	if rec.Level != slog.LevelInfo {
		t.Errorf("level = %v; want Info", rec.Level)
	}
	rec.Attrs(func(a slog.Attr) bool {
		t.Errorf("unexpected attr %s=%v on clean teardown; want no failed_steps/skipped_steps", a.Key, a.Value)
		return true
	})
}

func TestDestroySteps_SkipPath(t *testing.T) {
	h := &testutil.CaptureHandler{}
	defs := newPhaseWithCapture(h).destroySteps(context.Background(), minimalConfig(), minimalOpts())

	for i := 0; i < 4; i++ {
		if defs[i].SkipWhen == nil {
			t.Fatalf("defs[%d] has nil SkipWhen", i)
		}
		defs[i].SkipWhen()
	}

	if err := defs[4].Exec(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rec, ok := h.Last()
	if !ok {
		t.Fatal("no log records captured")
	}
	if rec.Level != slog.LevelInfo {
		t.Errorf("level = %v; want Info", rec.Level)
	}

	skippedVal := attrValue(&rec, "skipped_steps")
	if skippedVal == "" {
		t.Fatal("skipped_steps attr missing from log record")
	}
	for _, label := range []string{"terraform", "iso removal", "file cleanup", "firewall"} {
		if !strings.Contains(skippedVal, label) {
			t.Errorf("skipped_steps attr %q missing label %q", skippedVal, label)
		}
	}
}

func TestDestroySteps_ISOSkipReasonNamesCause(t *testing.T) {
	proxmoxCfg := func() *config.Config {
		return &config.Config{Provider: config.ProviderConfig{Proxmox: &config.ProxmoxConfig{}}}
	}
	baseOpts := func() *Options {
		return &Options{SkipTerraform: true, SkipCleanup: true, SkipFirewall: true}
	}
	cases := []struct {
		name     string
		cfg      *config.Config
		opts     *Options
		tfFailed bool
		want     string
	}{
		{
			"keep-isos", proxmoxCfg(), func() *Options { o := baseOpts(); o.KeepISOs = true; return o }(), false,
			"iso removal disabled via --keep-isos",
		},
		{
			"no provider", minimalConfig(), baseOpts(), false,
			"no proxmox provider configured",
		},
		{
			"terraform failed", proxmoxCfg(), baseOpts(), true,
			"terraform destroy failed — live vms may still reference these isos",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &testutil.CaptureHandler{}
			defs := newPhaseWithCapture(h).destroySteps(context.Background(), tc.cfg, tc.opts)
			if tc.tfFailed {
				defs[0].OnError(errors.New("tf-fail"))
			}

			if !defs[1].SkipWhen() {
				t.Fatal("iso step must skip for this cause")
			}
			got := defs[1].SkipReasonFunc()
			if got != tc.want {
				t.Fatalf("resolved reason = %q, want %q", got, tc.want)
			}
			if strings.Contains(got, " or ") || strings.Contains(got, ",") {
				t.Errorf("reason %q reads as a disjunction; it must name only the fired cause", got)
			}

			if err := defs[4].Exec(context.Background()); (err != nil) != tc.tfFailed {
				t.Fatalf("summary err = %v, want error only when terraform failed", err)
			}
			rec, ok := h.Last()
			if !ok {
				t.Fatal("no log records captured")
			}
			val := attrValue(&rec, "skipped_steps")
			if !strings.Contains(val, "iso removal: "+tc.want) {
				t.Errorf("summary skipped_steps = %q; want entry %q", val, "iso removal: "+tc.want)
			}
		})
	}
}

func TestDestroySteps_PartialFailure(t *testing.T) {
	h := &testutil.CaptureHandler{}
	defs := newPhaseWithCapture(h).destroySteps(context.Background(), minimalConfig(), minimalOpts())

	cases := []struct {
		idx   int
		label string
		err   string
	}{
		{0, labelTerraformDestroy, "tf-fail"},
		{1, "iso removal", "iso-fail"},
		{2, "file cleanup", "cleanup-fail"},
		{3, "firewall cleanup", "fw-fail"},
	}
	for _, c := range cases {
		if defs[c.idx].OnError == nil {
			t.Fatalf("defs[%d].OnError is nil; want wired for %q", c.idx, c.label)
		}
		defs[c.idx].OnError(errors.New(c.err))
	}

	err := defs[4].Exec(context.Background())
	if err == nil {
		t.Fatal("expected non-nil error from summary when steps failed")
	}
	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Fatalf("err = %v; want *errtypes.ClusterError", err)
	}
	for _, c := range cases {
		if !strings.Contains(clusterErr.Err.Error(), c.err) {
			t.Errorf("joined error %q missing %q", clusterErr.Err.Error(), c.err)
		}
	}

	rec, ok := h.Last()
	if !ok {
		t.Fatal("no log records captured")
	}
	if rec.Level != slog.LevelWarn {
		t.Errorf("level = %v; want Warn", rec.Level)
	}

	stepsVal := attrValue(&rec, "failed_steps")
	for _, c := range cases {
		if !strings.Contains(stepsVal, c.label) {
			t.Errorf("failed_steps %q missing %q", stepsVal, c.label)
		}
	}
	if strings.Contains(stepsVal, "print summary") {
		t.Errorf("failed_steps %q should not contain untracked step label %q", stepsVal, "print summary")
	}
}
