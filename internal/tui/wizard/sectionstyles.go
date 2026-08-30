package wizard

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// SectionStyles is the shared style set for review-style summary screens:
// cyan section headers over dashed separators, fixed-width dim labels, and
// a thick separator ahead of a screen's action selector.
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
			Render(strings.Repeat("┄", width-4)),
		ThickSeparator: lipgloss.NewStyle().
			Foreground(tui.ColorSlate600).
			Render(strings.Repeat("═", width-4)),
		Label: lipgloss.NewStyle().
			Foreground(tui.ColorSlate400).
			Width(18),
		Value: lipgloss.NewStyle().
			Foreground(tui.ColorText),
		Check: lipgloss.NewStyle().
			Foreground(tui.ColorSuccess),
	}
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
	var b strings.Builder
	b.WriteString(st.Header.Render(title))
	b.WriteString("\n")
	b.WriteString(st.Separator)
	b.WriteString("\n")
	for _, e := range visible {
		b.WriteString(st.KVPair(e.Label, e.Value))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}
