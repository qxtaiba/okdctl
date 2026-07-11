package render

import (
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
)

func TestBuilder(t *testing.T) {
	sb := NewBuilder()
	sb.WriteString("\n")
	sb.Section("api")
	sb.KV("reachable", "yes")
	sb.KVHighlight("username", "kubeadmin")
	sb.Newline()

	out := sb.String()
	for _, want := range []string{"api", "reachable", "yes", "username", "kubeadmin"} {
		if !strings.Contains(out, want) {
			t.Errorf("Builder output missing %q:\n%s", want, out)
		}
	}
}

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

func TestDryRunSummary(t *testing.T) {
	out := DryRunSummary("deploy step listing", []DryRunStep{{ID: "step-1", Name: "First step"}})
	for _, want := range []string{"dry-run — no changes made", "would execute", "step-1", "First step"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run summary missing %q:\n%s", want, out)
		}
	}
}
