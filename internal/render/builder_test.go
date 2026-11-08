package render

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
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
