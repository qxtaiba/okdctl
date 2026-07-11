package cli

import (
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/doctor"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// doctorJSONCheck is one entry in the JSON output's checks array. For
// multi-item checks, every sub-item becomes its own entry; detail is omitted
// when empty.
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
	// Runtime gate rather than a build tag so the check pipeline compiles,
	// lints, and tests on darwin dev hosts; doctor still refuses to run there.
	if runtime.GOOS != "linux" {
		return fmt.Errorf("okdctl doctor is only supported on linux (current: %s)", runtime.GOOS)
	}
	if err := validateFormat(doctorOutput); err != nil {
		return err
	}
	quietForJSON(doctorOutput)

	ctx := cmd.Context()

	checks := doctor.Checks(cfgFile)

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
		if fails > 0 {
			return &errtypes.ConfigError{Msg: "preflight checks failed"}
		}
		return nil
	}

	w := cmd.OutOrStdout()
	defer fmt.Fprintln(w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "🩺 "+tui.HighlightStyle.Render(fmt.Sprintf("doctor: running %d environment checks", len(checks))))
	fmt.Fprintln(w)

	for _, cr := range results {
		printResult(cr.c, cr.r, w)
	}

	switch {
	case fails > 0:
		tui.Warn("doctor: failing checks block deploy", tui.LF("failing", fails), tui.LF("warnings", warns))
		return &errtypes.ConfigError{Msg: "preflight checks failed"}
	case warns > 0:
		tui.Warn("doctor: deploy may proceed but review warnings above", tui.LF("warnings", warns))
	default:
		tui.Info("doctor: environment looks ready")
	}
	return nil
}

// severityMarkers returns the styled icon, the styled bracketed label,
// and the raw (unstyled) label text for a given severity. Callers use the
// raw text for column-width math when rendering aligned sub-item lists.
func severityMarkers(sev doctor.Severity) (icon, label, rawLabel string) {
	rawLabel = "[" + sev.String() + "]"
	switch sev {
	case doctor.Pass:
		icon = tui.SuccessStyle.Render("✓")
		label = tui.SuccessStyle.Render(rawLabel)
	case doctor.Warn:
		icon = tui.WarningStyle.Render("⚠")
		label = tui.WarningStyle.Render(rawLabel)
	case doctor.Fail:
		icon = tui.ErrorStyle.Render("✗")
		label = tui.ErrorStyle.Render(rawLabel)
	}
	return
}

// printResult renders one check as either a two-line block (title + single
// result line) or a title followed by a per-item sub-list. A blank line
// follows either shape so check blocks remain visually distinct.
func printResult(c doctor.Check, r doctor.Result, w io.Writer) {
	icon, aggregateLabel, _ := severityMarkers(r.Sev)

	title := c.Name
	if c.Desc != "" {
		title += tui.MutedStyle.Render(": " + c.Desc)
	}
	fmt.Fprintln(w, "  "+icon+" "+title)

	if len(r.Items) > 0 {
		// Sub-list: each item on its own line, labels aligned to the
		// widest possible label ("[fail]" / "[warn]" at 6 chars).
		const maxLabelWidth = 6
		for _, item := range r.Items {
			_, itemLabel, itemRawLabel := severityMarkers(item.Sev)
			padding := strings.Repeat(" ", maxLabelWidth-len(itemRawLabel)+2)
			line := "      " + itemLabel + padding + item.Name
			if item.Note != "" {
				line += tui.MutedStyle.Render(" (" + item.Note + ")")
			}
			fmt.Fprintln(w, line)
		}
	} else {
		fmt.Fprintln(w, "      "+aggregateLabel+" "+r.Detail)
	}

	fmt.Fprintln(w)
}
