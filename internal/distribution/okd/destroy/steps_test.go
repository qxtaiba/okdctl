package destroy

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
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

	var skippedVal string
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == "skipped_steps" {
			skippedVal = a.Value.String()
			return false
		}
		return true
	})
	if skippedVal == "" {
		t.Fatal("skipped_steps attr missing from log record")
	}
	for _, label := range []string{"terraform", "iso removal", "file cleanup", "firewall"} {
		if !strings.Contains(skippedVal, label) {
			t.Errorf("skipped_steps attr %q missing label %q", skippedVal, label)
		}
	}
}

// TestDestroySteps_ISOSkipReasonNamesCause locks that each distinct ISO-skip
// cause resolves to its own reason — not the historical or-list — and that
// the summary's skipped_steps entry carries the same resolved reason.
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
			var val string
			rec.Attrs(func(a slog.Attr) bool {
				if a.Key == "skipped_steps" {
					val = a.Value.String()
					return false
				}
				return true
			})
			if !strings.Contains(val, "iso removal: "+tc.want) {
				t.Errorf("summary skipped_steps = %q; want entry %q", val, "iso removal: "+tc.want)
			}
		})
	}
}

// TestDestroySteps_SkipReasonReachesStepLogAndSummary drives the real
// orchestrator over the built steps and pins that the per-step skip log line
// and the final summary both carry the resolved single-cause reason.
func TestDestroySteps_SkipReasonReachesStepLogAndSummary(t *testing.T) {
	h := &testutil.CaptureHandler{}
	p := newPhaseWithCapture(h)
	opts := &Options{SkipTerraform: true, SkipCleanup: true, SkipFirewall: true}
	defs := p.destroySteps(context.Background(), minimalConfig(), opts)

	orch := distribution.NewOrchestrator(distribution.BuildSteps(defs)...)
	orch.SetLogger(slog.New(h))
	if err := orch.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	const wantReason = "no proxmox provider configured"
	var stepLogReason, summarySkipped string
	for _, rec := range h.Records {
		var step, reason string
		rec.Attrs(func(a slog.Attr) bool {
			switch a.Key {
			case "step":
				step = a.Value.String()
			case "reason":
				reason = a.Value.String()
			case "skipped_steps":
				summarySkipped = a.Value.String()
			}
			return true
		})
		if step == string(StepRemoveRemoteISO) && reason != "" {
			stepLogReason = reason
		}
	}
	if stepLogReason != wantReason {
		t.Errorf("step log reason = %q, want %q", stepLogReason, wantReason)
	}
	if !strings.Contains(summarySkipped, "iso removal: "+wantReason) {
		t.Errorf("summary skipped_steps = %q; want entry %q", summarySkipped, "iso removal: "+wantReason)
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

	var stepsVal string
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == "failed_steps" {
			stepsVal = a.Value.String()
			return false
		}
		return true
	})
	for _, c := range cases {
		if !strings.Contains(stepsVal, c.label) {
			t.Errorf("failed_steps %q missing %q", stepsVal, c.label)
		}
	}
	if strings.Contains(stepsVal, "print summary") {
		t.Errorf("failed_steps %q should not contain untracked step label %q", stepsVal, "print summary")
	}
}
