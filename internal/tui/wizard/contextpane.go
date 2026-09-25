package wizard

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// answeredStep is implemented by steps that can summarize their current
// values as facts for the context pane's CONFIGURED section; a step without
// one contributes nothing there.
type answeredStep interface {
	Answered() []render.Fact
}

// focusedFieldStep is implemented by steps that can report their form's
// currently focused field's label and help text for the context pane's
// FOCUSED FIELD section.
type focusedFieldStep interface {
	FocusedFieldHelp() (label, help string, ok bool)
}

// renderContextPane renders the wide-terminal split layout's right-hand
// sidebar within exactly height rows: the step list (never dropped, though
// it may itself truncate under an extreme height budget), then the current
// step's answered facts, then the focused field's help text — a strict
// priority cascade, not three independent fit checks: FOCUSED FIELD may
// render only if CONFIGURED either rendered or was absent by interface,
// never after CONFIGURED was dropped for space, the same
// essentials-last-dropped order the footer's help ribbon uses. The pane
// stays subordinate to the form — most rows are muted — but carries real
// hierarchy: green checks on passed steps, one bright current row, section
// headers in the house label style.
func (m *Model) renderContextPane(width, height int) string {
	p := m.progressInfo()
	lines := paneStepsLines(p.Titles, p.Current-1, width, height)
	remaining := height - len(lines)

	// cascadeOK tracks whether every higher-priority section so far either
	// rendered or had nothing to render — the moment one is dropped for
	// space, every section after it in priority order is skipped outright,
	// regardless of whether its own content would have fit on its own.
	cascadeOK := true

	if facts := m.currentStepAnswered(); len(facts) > 0 {
		factLines := paneFactsLines(width, facts)
		if need := len(factLines) + 1; remaining >= need {
			lines = append(lines, "")
			lines = append(lines, factLines...)
			remaining -= need
		} else {
			cascadeOK = false
		}
	}

	if cascadeOK {
		if label, help, ok := m.currentStepFocusedFieldHelp(); ok {
			focusLines := paneFocusedFieldLines(width, label, help)
			if need := len(focusLines) + 1; remaining >= need {
				lines = append(lines, "")
				lines = append(lines, focusLines...)
			}
		}
	}

	return strings.Join(lines, "\n")
}

// paneStepsLines renders the PROGRESS section — a header row plus one row
// per visible step, prefixed with tui.IconSuccess/IconActive/IconPending for
// steps already passed, the current one, and the ones still ahead (the same
// three-tier status the header's own progress trail uses) — never exceeding
// maxHeight rows, and keeping the current step's row over any other once
// there's room for at least one row beside the header. The full list always
// fits above splitMinHeight, the gate composeWideBody checks before calling
// this at all; the sub-3-row path that can drop even the current row (down
// to the bare header at maxHeight 1-2) is an unreachable defensive floor,
// since that gate guarantees maxHeight is at least 1+stepCount.
func paneStepsLines(titles []string, current, width, maxHeight int) []string {
	header := paneSectionHeader("progress")
	if maxHeight <= 0 {
		return nil
	}

	rows := make([]string, len(titles))
	for i, title := range titles {
		glyph, label := paneStepStyles(i, current)
		rows[i] = glyph.Render(paneStepIcon(i, current)) + " " + label.Render(truncateTitle(title, width-2))
	}

	if 1+len(rows) <= maxHeight {
		return append([]string{header}, rows...)
	}

	budget := maxHeight - 1 // rows available under the header
	if budget <= 0 {
		return []string{header}
	}
	slots := budget - 1 // one row reserved for the "…" marker below
	if slots <= 0 {
		return []string{header}
	}

	keep := make(map[int]bool, slots)
	if current >= 0 && current < len(rows) {
		keep[current] = true
	}
	for radius := 1; len(keep) < slots; radius++ {
		added := false
		if i := current - radius; i >= 0 && i < len(rows) && !keep[i] {
			keep[i] = true
			added = true
		}
		if len(keep) < slots {
			if i := current + radius; i >= 0 && i < len(rows) && !keep[i] {
				keep[i] = true
				added = true
			}
		}
		if !added {
			break
		}
	}

	lines := []string{header}
	for i, row := range rows {
		if keep[i] {
			lines = append(lines, row)
		}
	}
	lines = append(lines, paneEllipsisRow())
	return lines
}

// paneEllipsisRow renders the dim marker paneStepsLines substitutes for
// whatever steps a too-small height budget couldn't fit.
func paneEllipsisRow() string {
	return lipgloss.NewStyle().Foreground(tui.ColorSlate600).Render("…")
}

// paneStepIcon returns the status glyph for step i relative to current: past
// steps get IconSuccess, the current step IconActive, the rest IconPending.
func paneStepIcon(i, current int) string {
	switch {
	case i < current:
		return tui.IconSuccess
	case i == current:
		return tui.IconActive
	default:
		return tui.IconPending
	}
}

// paneStepStyles returns the glyph and label styles for step i relative to
// current: a passed step's glyph is the real success green over a muted
// label, the current step renders bright with its glyph in the active
// accent, and pending steps stay dim throughout — so the list carries the
// same status hierarchy the form does without competing with it.
func paneStepStyles(i, current int) (glyph, label lipgloss.Style) {
	switch {
	case i < current:
		return lipgloss.NewStyle().Foreground(tui.ColorSuccess),
			lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	case i == current:
		return lipgloss.NewStyle().Foreground(tui.ColorPrimary),
			lipgloss.NewStyle().Foreground(tui.ColorText)
	default:
		return lipgloss.NewStyle().Foreground(tui.ColorSlate600),
			lipgloss.NewStyle().Foreground(tui.ColorSlate600)
	}
}

// currentStepAnswered returns the active step's answered facts, or nil when
// it doesn't implement answeredStep.
func (m *Model) currentStepAnswered() []render.Fact {
	if len(m.steps) == 0 || m.currentStep < 0 || m.currentStep >= len(m.steps) {
		return nil
	}
	ar, ok := m.steps[m.currentStep].(answeredStep)
	if !ok {
		return nil
	}
	return ar.Answered()
}

// paneFactsLines renders the CONFIGURED section's lines — a header row plus
// one "key: value" row per fact, wrapped to width — without padding them to
// any particular height; the caller decides whether they fit.
func paneFactsLines(width int, facts []render.Fact) []string {
	lines := make([]string, 0, len(facts)+1)
	lines = append(lines, paneSectionHeader("configured"))
	for _, f := range facts {
		lines = append(lines, paneFactLines(width, f)...)
	}
	return lines
}

// paneFactLines renders one fact as wrapped rows with the key dim and the
// value in normal body text. The raw "key: value" line is wrapped first and
// styled after (styling before wrapping would split ANSI sequences), so the
// key/value seam is re-found by rune count on each wrapped row.
func paneFactLines(width int, f render.Fact) []string {
	keyStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	valueStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate300)

	keyLen := len([]rune(f.Key + ":"))
	consumed := 0
	var out []string
	for line := range strings.SplitSeq(lipgloss.Wrap(f.Key+": "+f.Value, width, ""), "\n") {
		runes := []rune(line)
		switch {
		case consumed >= keyLen:
			out = append(out, valueStyle.Render(line))
		case len(runes) <= keyLen-consumed:
			out = append(out, keyStyle.Render(line))
		default:
			seam := keyLen - consumed
			out = append(out, keyStyle.Render(string(runes[:seam]))+valueStyle.Render(string(runes[seam:])))
		}
		consumed += len(runes)
	}
	return out
}

// currentStepFocusedFieldHelp returns the active step's focused field's
// label and help text, or ok=false when it doesn't implement
// focusedFieldStep or has no focused field.
func (m *Model) currentStepFocusedFieldHelp() (label, help string, ok bool) {
	if len(m.steps) == 0 || m.currentStep < 0 || m.currentStep >= len(m.steps) {
		return "", "", false
	}
	fr, isFocusReporter := m.steps[m.currentStep].(focusedFieldStep)
	if !isFocusReporter {
		return "", "", false
	}
	return fr.FocusedFieldHelp()
}

// paneFocusedFieldLines renders the FOCUSED FIELD section's lines — a header
// row, the field's label in normal body text, then its help text in dim
// italic, both wrapped to width — without padding them to any particular
// height; the caller decides whether they fit.
func paneFocusedFieldLines(width int, label, help string) []string {
	labelLine := lipgloss.NewStyle().Foreground(tui.ColorSlate300).Render(lipgloss.Wrap(label, width, ""))
	helpLine := lipgloss.NewStyle().Foreground(tui.ColorSlate500).Italic(true).Render(lipgloss.Wrap(help, width, ""))

	lines := []string{paneSectionHeader("focused field")}
	lines = append(lines, strings.Split(labelLine, "\n")...)
	lines = append(lines, strings.Split(helpLine, "\n")...)
	return lines
}

// paneSectionHeader renders an uppercased section label in the house
// subsection style — the same cyan bold SectionStyles.Header gives the
// review screen's section titles — so the pane's sections read as proper
// labels rather than more dim body text.
func paneSectionHeader(title string) string {
	return lipgloss.NewStyle().Foreground(tui.ColorCyan500).Bold(true).Render(strings.ToUpper(title))
}
