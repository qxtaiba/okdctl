package cli

import (
	"bytes"
	"errors"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/doctor"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/tui"
)

func TestDoctorExitErr_ExitCodeMapping(t *testing.T) {
	cases := []struct {
		name  string
		fails int
		warns int
		want  int
	}{
		{"clean", 0, 0, 0},
		{"warn only", 0, 1, 6},
		{"fail beats warn", 1, 1, 2},
		{"fail only", 2, 0, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exitCodeFor(doctorExitErr(tc.fails, tc.warns)); got != tc.want {
				t.Errorf("exit code = %d; want %d", got, tc.want)
			}
		})
	}
}

// TestRunDoctorNonLinuxGateIsUsageError is meaningful only off linux, where the
// OS gate is reachable.
func TestRunDoctorNonLinuxGateIsUsageError(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("gate is unreachable on linux")
	}
	err := runDoctor(doctorCmd, nil)
	var usageErr *errtypes.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("want *errtypes.UsageError (exit 64), got %T: %v", err, err)
	}
}

// severityLabelPrefix matches a row's leading indent, severity badge, and
// column padding, so the match length gives the column where row text starts.
var severityLabelPrefix = regexp.MustCompile(`^\s*\[(?:ok|warn|fail)\]\s*`)

// TestPrintResultAlignsAggregateAndItemColumns pins a bytes.Buffer non-TTY
// profile so byte-offset column math stays valid on ANSI-styled rows.
func TestPrintResultAlignsAggregateAndItemColumns(t *testing.T) {
	tui.SetColorProfileFor(&bytes.Buffer{})
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })

	labelWidth := severityLabelWidth()
	check := doctor.Check{Name: "check"}

	results := []doctor.Result{
		{Sev: doctor.Pass, Detail: "aggregate ok detail"},
		{Sev: doctor.Warn, Detail: "aggregate warn detail"},
		{Sev: doctor.Fail, Detail: "aggregate fail detail"},
		{Items: []doctor.Item{{Sev: doctor.Pass, Name: "item ok name"}}},
		{Items: []doctor.Item{{Sev: doctor.Warn, Name: "item warn name"}}},
		{Items: []doctor.Item{{Sev: doctor.Fail, Name: "item fail name"}}},
	}

	var starts []int
	for _, r := range results {
		var buf bytes.Buffer
		printResult(check, r, labelWidth, &buf)
		lines := strings.Split(buf.String(), "\n")
		if len(lines) < 2 {
			t.Fatalf("printResult wrote %d lines, want at least a title and a result row:\n%q", len(lines), buf.String())
		}
		loc := severityLabelPrefix.FindStringIndex(lines[1])
		if loc == nil {
			t.Fatalf("row %q does not start with a severity label", lines[1])
		}
		starts = append(starts, loc[1])
	}

	for i, s := range starts {
		if s != starts[0] {
			t.Errorf("result %d: row text starts at column %d; want %d (aggregate and item rows must share one label column)", i, s, starts[0])
		}
	}
}

// TestPrintResultNoANSIUnderNoColor pins a bytes.Buffer non-TTY profile so
// Downsample strips every escape rather than downgrading it.
func TestPrintResultNoANSIUnderNoColor(t *testing.T) {
	tui.SetColorProfileFor(&bytes.Buffer{})
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })

	check := doctor.Check{Name: "check", Desc: "desc"}
	result := doctor.Result{
		Sev: doctor.Warn,
		Items: []doctor.Item{
			{Sev: doctor.Pass, Name: "ok item"},
			{Sev: doctor.Warn, Name: "warn item", Note: "needs attention"},
			{Sev: doctor.Fail, Name: "fail item"},
		},
	}

	var buf bytes.Buffer
	printResult(check, result, severityLabelWidth(), &buf)

	if strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("doctor output leaked ANSI escapes under no-color:\n%q", buf.String())
	}
}
