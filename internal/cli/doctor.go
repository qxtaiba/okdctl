package cli

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/clusterstatus"
	"github.com/qxtaiba/okdctl/internal/doctor"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// errDoctorWarn is a warn-only sentinel (not errtypes) mapped to exit code 6;
// see docs/cli/exit-codes.md.
var errDoctorWarn = errors.New("doctor: warnings present, no failures")

// doctorExitErr maps fail/warn tallies to runDoctor's return value; shared by
// the JSON and text paths so they can't drift on exit code.
func doctorExitErr(fails, warns int) error {
	switch {
	case fails > 0:
		return &errtypes.ConfigError{Msg: "preflight checks failed"}
	case warns > 0:
		return errDoctorWarn
	default:
		return nil
	}
}

// doctorJSONCheck is one entry in the JSON output's checks array; a multi-item
// check emits one entry per sub-item.
type doctorJSONCheck struct {
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Detail   string `json:"detail,omitempty"`
}

// doctorJSONOutput is the top-level envelope emitted by --output=json.
type doctorJSONOutput struct {
	Checks []doctorJSONCheck `json:"checks"`
	Failed int               `json:"failed"`
	Warned int               `json:"warned"`
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	// Runtime gate (not a build tag) keeps the pipeline compiling/testing on darwin dev hosts.
	if runtime.GOOS != "linux" {
		return &errtypes.UsageError{Msg: fmt.Sprintf("okdctl doctor is only supported on linux (current: %s)", runtime.GOOS)}
	}
	if err := validateFormat(doctorOutput); err != nil {
		return err
	}
	quietForJSON(doctorOutput)

	ctx := cmd.Context()

	checks := doctor.Checks(cfgFile)
	if cc, ok := discoverClusterCheck(); ok {
		checks = append(checks, cc)
	}

	type collectedResult struct {
		c doctor.Check
		r doctor.Result
	}
	results := make([]collectedResult, 0, len(checks))
	var fails, warns int
	for _, c := range checks {
		r := c.Fn(ctx)
		results = append(results, collectedResult{c, r})
		switch r.Sev {
		case doctor.Fail:
			fails++
		case doctor.Warn:
			warns++
		}
	}

	if doctorOutput == outputJSON {
		var jsonChecks []doctorJSONCheck
		for _, cr := range results {
			if len(cr.r.Items) > 0 {
				for _, item := range cr.r.Items {
					entry := doctorJSONCheck{
						Name:     cr.c.Name + "/" + item.Name,
						Severity: item.Sev.String(),
					}
					if item.Note != "" {
						entry.Detail = item.Note
					}
					jsonChecks = append(jsonChecks, entry)
				}
			} else {
				jsonChecks = append(jsonChecks, doctorJSONCheck{
					Name:     cr.c.Name,
					Severity: cr.r.Sev.String(),
					Detail:   cr.r.Detail,
				})
			}
		}
		out := doctorJSONOutput{Checks: jsonChecks, Failed: fails, Warned: warns}
		if encErr := writeJSON(cmd.OutOrStdout(), out); encErr != nil {
			return encErr
		}
		return doctorExitErr(fails, warns)
	}

	w := cmd.OutOrStdout()
	defer fmt.Fprintln(w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, tui.SubsectionLabel(fmt.Sprintf("doctor: running %d environment checks", len(checks))))
	fmt.Fprintln(w)

	labelWidth := severityLabelWidth()
	for _, cr := range results {
		printResult(cr.c, cr.r, labelWidth, w)
	}

	switch {
	case fails > 0:
		logutil.Warn("doctor: failing checks block deploy", logutil.LF("failing", fails), logutil.LF("warnings", warns))
	case warns > 0:
		logutil.Warn("doctor: deploy may proceed but review warnings above", logutil.LF("warnings", warns))
	default:
		logutil.Info("doctor: environment looks ready")
	}
	return doctorExitErr(fails, warns)
}

// discoverClusterCheck returns the day-2 cluster check when a kubeconfig is
// present, else ok=false so doctor stays a pure pre-deploy tool.
func discoverClusterCheck() (doctor.Check, bool) {
	root, err := resolveProjectRoot()
	if err != nil {
		return doctor.Check{}, false
	}
	cl, err := clusterstatus.NewClient(root)
	if err != nil {
		return doctor.Check{}, false
	}
	return doctor.ClusterCheck(cl), true
}

// severityMarkers returns the styled icon, styled label, and raw label text;
// callers use raw text for column-width math.
func severityMarkers(sev doctor.Severity) (icon, label, rawLabel string) {
	rawLabel = "[" + sev.String() + "]"
	switch sev {
	case doctor.Pass:
		icon = tui.SuccessStyle.Render(tui.IconSuccess)
		label = tui.SuccessStyle.Render(rawLabel)
	case doctor.Warn:
		icon = tui.WarningStyle.Render(tui.IconWarning)
		label = tui.WarningStyle.Render(rawLabel)
	case doctor.Fail:
		icon = tui.ErrorStyle.Render(tui.IconError)
		label = tui.ErrorStyle.Render(rawLabel)
	}
	return
}

// severityLabelWidth returns the render width of the widest severity badge
// ("[ok]"/"[warn]"/"[fail]") so aggregate and item rows share one label column.
func severityLabelWidth() int {
	width := 0
	for _, sev := range []doctor.Severity{doctor.Pass, doctor.Warn, doctor.Fail} {
		width = max(width, lipgloss.Width("["+sev.String()+"]"))
	}
	return width
}

// padSeverityLabel right-pads a rendered severity label to width using raw's
// unstyled length, so text after the label lands on the same column
// regardless of which severity rendered it.
func padSeverityLabel(label, raw string, width int) string {
	return label + strings.Repeat(" ", width-lipgloss.Width(raw)+2)
}

func printResult(c doctor.Check, r doctor.Result, labelWidth int, w io.Writer) {
	icon, aggregateLabel, aggregateRawLabel := severityMarkers(r.Sev)

	title := c.Name
	if c.Desc != "" {
		title += tui.MutedStyle.Render(": " + c.Desc)
	}
	fmt.Fprintln(w, tui.Downsample("  "+icon+" "+title))

	if len(r.Items) > 0 {
		for _, item := range r.Items {
			_, itemLabel, itemRawLabel := severityMarkers(item.Sev)
			line := "      " + padSeverityLabel(itemLabel, itemRawLabel, labelWidth) + item.Name
			if item.Note != "" {
				line += tui.MutedStyle.Render(" (" + item.Note + ")")
			}
			fmt.Fprintln(w, tui.Downsample(line))
		}
	} else {
		fmt.Fprintln(w, tui.Downsample("      "+padSeverityLabel(aggregateLabel, aggregateRawLabel, labelWidth)+r.Detail))
	}

	fmt.Fprintln(w)
}
