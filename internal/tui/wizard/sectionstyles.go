package wizard

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// minLabelWidth is the narrowest a section's label column ever shrinks to.
const minLabelWidth = 12

// SectionStyles is the shared style set for review-style summary screens:
// cyan section headers over full-width separators, a label column fitted to
// its section's content, and a thick separator ahead of a screen's action
// selector.
type SectionStyles struct {
	Header         lipgloss.Style
	Separator      string
	ThickSeparator string
	Label          lipgloss.Style
	Value          lipgloss.Style
	Check          lipgloss.Style
}

// NewSectionStyles builds the section styles sized to the step's width.
func NewSectionStyles(width int) SectionStyles {
	return SectionStyles{
		Header: lipgloss.NewStyle().
			Foreground(tui.ColorCyan500).
			Bold(true),
		Separator: lipgloss.NewStyle().
			Foreground(tui.ColorSlate700).
			Render(strings.Repeat("┄", width)),
		ThickSeparator: lipgloss.NewStyle().
			Foreground(tui.ColorSlate600).
			Render(strings.Repeat("═", width)),
		Label: lipgloss.NewStyle().
			Foreground(tui.ColorSlate400).
			Width(minLabelWidth),
		Value: lipgloss.NewStyle().
			Foreground(tui.ColorText),
		Check: lipgloss.NewStyle().
			Foreground(tui.ColorSuccess),
	}
}

// LabelWidthFor returns the label column width that fits the longest of
// labels plus two columns of padding, never narrower than minLabelWidth.
func LabelWidthFor(labels ...string) int {
	w := minLabelWidth
	for _, label := range labels {
		if lw := lipgloss.Width(label) + 2; lw > w {
			w = lw
		}
	}
	return w
}

// NewSectionStylesFor builds section styles sized to width with the label
// column fitted to labels.
func NewSectionStylesFor(width int, labels ...string) SectionStyles {
	return NewSectionStyles(width).ForLabels(labels...)
}

// ForLabels returns a copy of st with the label column fitted to labels.
func (st SectionStyles) ForLabels(labels ...string) SectionStyles { //nolint:gocritic // hugeParam: value receiver is deliberate, it returns an independent copy so callers never mutate a shared SectionStyles
	st.Label = st.Label.Width(LabelWidthFor(labels...))
	return st
}

// KVPair renders one label/value line using the section label column.
func (st *SectionStyles) KVPair(label, value string) string {
	return st.Label.Render(label) + st.Value.Render(value)
}

// KVEntry describes one label/value line; Skip omits it entirely rather than rendering blank.
type KVEntry struct {
	Label string
	Value string
	Skip  bool
}

// RenderSection emits a titled block of KVEntry lines, or "" if every entry is skipped.
func RenderSection(st *SectionStyles, title string, entries []KVEntry) string {
	visible := entries[:0:0]
	for _, e := range entries {
		if !e.Skip {
			visible = append(visible, e)
		}
	}
	if len(visible) == 0 {
		return ""
	}
	labels := make([]string, len(visible))
	for i, e := range visible {
		labels[i] = e.Label
	}
	fitted := st.ForLabels(labels...)

	var b strings.Builder
	b.WriteString(fitted.Header.Render(title))
	b.WriteString("\n")
	b.WriteString(fitted.Separator)
	b.WriteString("\n")
	for _, e := range visible {
		b.WriteString(fitted.KVPair(e.Label, e.Value))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}
