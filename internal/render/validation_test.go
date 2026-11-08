package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
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

func TestValidationSummaryNoANSIUnderNoColor(t *testing.T) {
	tui.SetColorProfileFor(&bytes.Buffer{}) // a buffer is never a TTY
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })

	invalid := &config.ValidationResult{}
	invalid.AddError("cluster.name", "must not be empty")
	out := ValidationSummary(invalid)

	if strings.Contains(out, "\x1b[") {
		t.Errorf("ValidationSummary leaked ANSI escapes under a no-color profile:\n%q", out)
	}
}
