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

func TestDestroySteps_FailurePath(t *testing.T) {
	h := &testutil.CaptureHandler{}
	defs := newPhaseWithCapture(h).destroySteps(context.Background(), minimalConfig(), minimalOpts())

	tracked := []struct {
		idx   int
		id    distribution.StepID
		label string
	}{
		{0, StepDestroyInfra, labelTerraformDestroy},
		{1, StepRemoveRemoteISO, "iso removal"},
		{2, StepCleanupFiles, "file cleanup"},
		{3, StepCleanupFirewall, "firewall cleanup"},
	}

	sentinel := errors.New("sentinel")
	for _, tc := range tracked {
		if defs[tc.idx].ID != tc.id {
			t.Fatalf("defs[%d].ID = %q; want %q", tc.idx, defs[tc.idx].ID, tc.id)
		}
		if defs[tc.idx].OnError == nil {
			t.Fatalf("defs[%d] (%s) has nil OnError", tc.idx, tc.id)
		}
		defs[tc.idx].OnError(sentinel)
	}

	if defs[4].ID != StepPrintSummary {
		t.Fatalf("defs[4].ID = %q; want %q", defs[4].ID, StepPrintSummary)
	}
	err := defs[4].Exec(context.Background())
	if err == nil {
		t.Fatal("expected non-nil error from summary when steps failed")
	}
	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Fatalf("err = %v; want *errtypes.ClusterError", err)
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
	for _, tc := range tracked {
		if !strings.Contains(stepsVal, tc.label) {
			t.Errorf("failed_steps attr %q missing label %q", stepsVal, tc.label)
		}
	}
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

func TestDestroySteps_PartialFailure(t *testing.T) {
	h := &testutil.CaptureHandler{}
	defs := newPhaseWithCapture(h).destroySteps(context.Background(), minimalConfig(), minimalOpts())

	defs[0].OnError(errors.New("tf-fail"))
	defs[3].OnError(errors.New("fw-fail"))

	err := defs[4].Exec(context.Background())
	if err == nil {
		t.Fatal("expected non-nil error from summary when steps failed")
	}
	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Fatalf("err = %v; want *errtypes.ClusterError", err)
	}
	if !strings.Contains(clusterErr.Err.Error(), "tf-fail") {
		t.Errorf("joined error %q missing 'tf-fail'", clusterErr.Err.Error())
	}
	if !strings.Contains(clusterErr.Err.Error(), "fw-fail") {
		t.Errorf("joined error %q missing 'fw-fail'", clusterErr.Err.Error())
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
	if !strings.Contains(stepsVal, labelTerraformDestroy) {
		t.Errorf("failed_steps %q missing %q", stepsVal, labelTerraformDestroy)
	}
	if !strings.Contains(stepsVal, "firewall cleanup") {
		t.Errorf("failed_steps %q missing 'firewall cleanup'", stepsVal)
	}
	if strings.Contains(stepsVal, "iso removal") {
		t.Errorf("steps %q should not contain 'iso removal'", stepsVal)
	}
	if strings.Contains(stepsVal, "file cleanup") {
		t.Errorf("steps %q should not contain 'file cleanup'", stepsVal)
	}
}
