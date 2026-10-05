package render

import (
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestPostDeployFactsMatchTheSummaryBox(t *testing.T) {
	tui.SetTerminalWidth(200)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	cfg := config.DefaultConfig()
	result := &postinstall.Result{KubeVipIP: "192.168.1.50", BootstrapCleaned: true, DNSDeployed: true}
	steps := []distribution.StepResult{
		{StepID: "download-tools", Success: true, Duration: 3 * time.Second},
		{StepID: "deploy-infrastructure", Success: true, Duration: 90 * time.Second},
	}

	box := tuitest.StripANSI(PostDeploySummaryWidth(cfg, result, steps, "run-42", 120))
	facts := NewPostDeployFacts(cfg, result, steps)

	sections := map[string][]tui.FactRow{
		"access": facts.Access, "dns": facts.DNS,
		"status": facts.Status, "steps": facts.Steps, "credentials": facts.Credentials,
	}
	for name, rows := range sections {
		if len(rows) == 0 {
			t.Errorf("%s section came back empty", name)
		}
		for _, row := range rows {
			if !strings.Contains(box, row.Key) {
				t.Errorf("%s: box does not carry key %q", name, row.Key)
			}
			if !strings.Contains(box, row.Value) {
				t.Errorf("%s: box does not carry value %q for %q", name, row.Value, row.Key)
			}
		}
	}
	for _, cmd := range facts.QuickStart {
		if !strings.Contains(box, cmd) {
			t.Errorf("box does not carry quick-start command %q", cmd)
		}
	}
}

func TestPostDeployFactsWithoutAPostinstallResultStayEmpty(t *testing.T) {
	facts := NewPostDeployFacts(config.DefaultConfig(), nil, nil)
	if len(facts.Status) != 0 {
		t.Errorf("status = %v, want empty with no postinstall result", facts.Status)
	}
	if len(facts.Steps) != 0 {
		t.Errorf("steps = %v, want empty with no step results", facts.Steps)
	}
	if len(facts.Access) == 0 {
		t.Error("access facts come from the config alone and must still be there")
	}
}
