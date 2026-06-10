package deploymetrics

import (
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution"
)

// TestRecorder_ZeroValueStepFinished verifies a zero-value Recorder does not
// panic when StepFinished writes its maps before any explicit initialisation.
func TestRecorder_ZeroValueStepFinished(t *testing.T) {
	var r Recorder
	result := &distribution.StepResult{
		StepID:   distribution.StepID("test-step"),
		Success:  true,
		Duration: time.Second,
	}
	r.StepFinished(result)
	if r.stepTotal["test-step"][0] != 1 {
		t.Fatalf("expected success count 1, got %d", r.stepTotal["test-step"][0])
	}
}
