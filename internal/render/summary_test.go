package render

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
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

func TestBuilderNoteAndBulletFit(t *testing.T) {
	tui.SetTerminalWidth(60)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	sb := NewBuilder()
	sb.Section("details")
	sb.Note("about", strings.Repeat("word ", 40))
	sb.Bullet(strings.Repeat("bullet-item ", 30))
	sb.Para(strings.Repeat("paragraph-text ", 30))
	sb.Newline()

	out := "\n" + tui.BoxedSectionCompact(sb.String(), "note and bullet", tui.DefaultBoxWidth) + "\n"
	tuitest.AssertFits(t, out, 60, 0)
}

// Builder.Table had no width coverage before its first real caller (the
// node-op tables): the widest realistic row (a real-length terraform
// address in a MaxColWidth:40 column) plus the Table's 4-space indent must
// still fit ContentWidth, both on a narrow 80-col terminal and at the box's
// default width.
func TestBuilderTableFitsContentWidth(t *testing.T) {
	headers := []string{"NODE", "ROLE", "ADDRESS", "ACTION"}
	rows := [][]string{
		{"grappleberry-worker2", "worker", "module.okd_cluster.proxmox_virtual_environment_vm.worker[2]", "resized"},
	}

	for _, w := range []int{80, tui.DefaultBoxWidth} {
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			tui.SetTerminalWidth(w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			sb := NewBuilder()
			sb.Table(headers, rows, tui.TableOptions{MaxColWidth: 40})

			for _, line := range strings.Split(strings.TrimRight(sb.String(), "\n"), "\n") {
				if lw := lipgloss.Width(line); lw > sb.ContentWidth() {
					t.Errorf("table row is %d cols, want <= ContentWidth %d: %q", lw, sb.ContentWidth(), line)
				}
			}
		})
	}
}

// SubKV's narrowed-column math is otherwise unverified: sibling SubKV rows
// must line up their values regardless of key length, and that shared
// column must sit 2 narrower than a plain KV row's.
func TestBuilderSubKVAlignedAndNarrowed(t *testing.T) {
	sb := NewBuilder()
	sb.SubKV("a", "alpha-value")
	sb.SubKV("much-longer-sub-key", "beta-value")
	sb.KV("plain", "control-value")

	lines := strings.Split(strings.TrimRight(sb.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 rendered lines, got %d:\n%q", len(lines), sb.String())
	}

	subCol0 := strings.Index(lines[0], "alpha-value")
	subCol1 := strings.Index(lines[1], "beta-value")
	kvCol := strings.Index(lines[2], "control-value")
	if subCol0 < 0 || subCol1 < 0 || kvCol < 0 {
		t.Fatalf("expected every value marker present:\n%s", sb.String())
	}

	if subCol0 != subCol1 {
		t.Errorf("SubKV values must align across sibling rows regardless of key length: %d vs %d", subCol0, subCol1)
	}
	if diff := kvCol - subCol0; diff != 2 {
		t.Errorf("SubKV's value column should start 2 columns earlier than a plain KV row: kv=%d sub=%d diff=%d", kvCol, subCol0, diff)
	}
}

func TestPostDeploySummaryGoldenAtWidths(t *testing.T) {
	cfg := config.DefaultConfig()
	result := &postinstall.Result{KubeVipIP: "192.168.1.50", BootstrapCleaned: true, DNSDeployed: true}
	steps := []distribution.StepResult{
		{StepID: "download-tools", Success: true, Duration: 3 * time.Second},
		{StepID: "deploy-infrastructure", Success: true, Duration: 90 * time.Second},
	}

	for _, w := range []int{60, 80, 100, 120} {
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			tui.SetTerminalWidth(w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			out := PostDeploySummary(cfg, result, steps, "run-42")
			tuitest.AssertFits(t, out, w, 0)
			tuitest.Golden(t, fmt.Sprintf("post_deploy_%d", w), out)
		})
	}
}
