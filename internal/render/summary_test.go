package render

import (
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
)

func TestValidationSummary(t *testing.T) {
	valid := &config.ValidationResult{}
	if out := ValidationSummary(valid); !strings.Contains(out, "configuration is valid") {
		t.Errorf("valid summary = %q; want it to contain configuration is valid", out)
	}

	invalid := &config.ValidationResult{}
	invalid.AddError("cluster.name", "must not be empty")
	out := ValidationSummary(invalid)
	for _, want := range []string{"configuration invalid (1 errors)", "cluster.name", "must not be empty"} {
		if !strings.Contains(out, want) {
			t.Errorf("invalid summary missing %q:\n%s", want, out)
		}
	}
}

func TestInterruptSummary(t *testing.T) {
	steps := []distribution.StepResult{
		{StepID: "download-tools", Success: true, Duration: 3 * time.Second},
		{StepID: "build-isos", Skipped: true},
	}
	out := InterruptSummary(steps, "okdctl deploy", "run-42")
	for _, want := range []string{"interrupted", "run-42", "download-tools", "ok", "build-isos", "skip", "okdctl deploy"} {
		if !strings.Contains(out, want) {
			t.Errorf("interrupt summary missing %q:\n%s", want, out)
		}
	}
}

func TestFailureSummary(t *testing.T) {
	steps := []distribution.StepResult{
		{StepID: "download-tools", Success: true, Duration: 3 * time.Second},
		{StepID: "deploy-infrastructure", Duration: 90 * time.Second},
	}
	out := FailureSummary(&FailureInfo{
		Steps:        steps,
		Phase:        "install",
		RunID:        "run-42",
		Elapsed:      41 * time.Minute,
		TeardownCmd:  "okdctl destroy",
		TeardownNote: "remove provisioned resources",
	})
	for _, want := range []string{
		"deploy failed", "run-42", "failed phase", "install",
		"failed step", "deploy-infrastructure", "elapsed", "41m0s",
		"download-tools", "ok", "fail",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("failure summary missing %q:\n%s", want, out)
		}
	}

	resume := strings.Index(out, "to resume from install")
	fresh := strings.Index(out, "--fresh")
	destroy := strings.Index(out, "okdctl destroy")
	if resume < 0 || fresh < 0 || destroy < 0 || resume > fresh || fresh > destroy {
		t.Errorf("next steps not ordered resume, --fresh, destroy (%d, %d, %d):\n%s", resume, fresh, destroy, out)
	}
}
