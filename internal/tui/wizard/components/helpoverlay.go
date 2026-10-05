package components

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// helpOverlayGlobalKeys are the KeyHint.Key values RenderHelpOverlay always
// buckets under "global" regardless of which step contributed them — they
// describe the wizard's own chrome (back, quit, scroll, the overlay
// itself), not the active step's fields. Mirrors wizard.HelpEsc,
// wizard.HelpCtrlC, wizard.HelpQuestion, and the footer's "pgup/pgdn" scroll
// hint; components cannot import the wizard package (which already imports
// components), so the literals are duplicated here deliberately.
var helpOverlayGlobalKeys = map[string]bool{
	"esc":       true,
	"ctrl+c":    true,
	"pgup/pgdn": true,
	"?":         true,
}

// helpOverlayVimKeys are the KeyHint.Key values bucketed under the overlay's
// "vim" section: the footer-silent scroll vocabulary the wizard binds on
// every scrollable pane.
var helpOverlayVimKeys = map[string]bool{
	"j/k":      true,
	"ctrl+d/u": true,
	"gg/G":     true,
}

// The overlay styles are assigned by rebuildHelpOverlayStyles (via
// RebuildStyles) so the background flip reaches them.
var (
	helpOverlayTitleStyle   lipgloss.Style
	helpOverlaySectionStyle lipgloss.Style
	helpOverlayKeyStyle     lipgloss.Style
	helpOverlayHintStyle    lipgloss.Style
	helpOverlayPanelStyle   lipgloss.Style
)

func rebuildHelpOverlayStyles() {
	helpOverlayTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(tui.ColorText())
	helpOverlaySectionStyle = lipgloss.NewStyle().Bold(true).Foreground(tui.ColorTextDim())
	helpOverlayKeyStyle = lipgloss.NewStyle().Bold(true).Foreground(tui.ColorTextSoft())
	helpOverlayHintStyle = lipgloss.NewStyle().Italic(true).Foreground(tui.ColorSubtle())
	helpOverlayPanelStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorSubtle()).
		Padding(0, 1)
}

// RenderHelpOverlay renders every binding in bindings — grouped into a
// "screen" section (the active step's own keys), a "vim" section (the
// footer-silent scroll vocabulary, per helpOverlayVimKeys), and a "global"
// section (back/quit/scroll/the overlay itself, per helpOverlayGlobalKeys)
// — as a centered panel sized to exactly width×height, wrapping the screen
// section into two columns once one would overflow height.
func RenderHelpOverlay(bindings []KeyHint, width, height int) string {
	screen, vim, global := partitionHelpBindings(bindings)

	innerWidth := max(width-4, 20)  // panel border(2) + padding(2)
	innerHeight := max(height-2, 1) // panel border only, no vertical padding

	body := renderHelpOverlayBody(screen, vim, global, innerWidth, innerHeight)
	panel := clampWidth(helpOverlayPanelStyle.Render(body), width)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel)
}

func partitionHelpBindings(bindings []KeyHint) (screen, vim, global []KeyHint) {
	for _, b := range bindings {
		switch {
		case helpOverlayGlobalKeys[b.Key]:
			global = append(global, b)
		case helpOverlayVimKeys[b.Key]:
			vim = append(vim, b)
		default:
			screen = append(screen, b)
		}
	}
	return screen, vim, global
}

// renderHelpOverlayBody lays out the title, the "screen" section (packed
// into two columns once it would otherwise overflow height), the "vim" and
// "global" sections (one labeled ribbon line each — both are always short),
// and a closing hint, joined as one block no taller than height. If the
// screen section still doesn't fit its budget — renderHelpColumns sizes to
// it by construction, but this is the backstop if that arithmetic ever
// drifts — rows are dropped from the screen section alone: head (the title)
// and tail (the vim/global ribbons, which carry ctrl+c/?, plus the closing
// hint) always survive intact, never the other way around.
func renderHelpOverlayBody(screen, vim, global []KeyHint, width, height int) string {
	title := helpOverlayTitleStyle.Render("key bindings")
	hint := helpOverlayHintStyle.Render("esc or ? closes")

	var head, tail []string
	head = append(head, title, "")
	if len(screen) > 0 {
		head = append(head, helpOverlaySectionStyle.Render("screen"))
	}
	if len(vim) > 0 {
		if len(screen) > 0 {
			tail = append(tail, "")
		}
		tail = append(tail, renderHelpSectionLines("vim", vim, width)...)
	}
	if len(global) > 0 {
		if len(screen) > 0 && len(vim) == 0 {
			tail = append(tail, "")
		}
		tail = append(tail, renderHelpSectionLines("global", global, width)...)
	}
	tail = append(tail, "", hint)

	screenBudget := max(height-len(head)-len(tail), 1)
	screenLines := renderHelpColumns(screen, width, screenBudget)
	if avail := height - len(head) - len(tail); avail >= 0 && len(screenLines) > avail {
		screenLines = screenLines[:avail]
	}

	lines := make([]string, 0, len(head)+len(tail)+len(screenLines))
	lines = append(lines, head...)
	lines = append(lines, screenLines...)
	lines = append(lines, tail...)

	return strings.Join(lines, "\n")
}

// renderHelpColumns renders items two per row (key/help pairs, left column
// padded to align) once one row per item would need more than maxRows,
// otherwise one per row. The left column sizes to its own widest entry
// rather than a blind half-split, so a long help string doesn't get clipped
// just because it landed on the left; only a panel too narrow for both
// columns' content clips anything, and only the overflowing side.
func renderHelpColumns(items []KeyHint, width, maxRows int) []string {
	if len(items) == 0 {
		return nil
	}
	if len(items) <= maxRows {
		lines := make([]string, len(items))
		for i, it := range items {
			lines[i] = "  " + formatHelpBinding(it)
		}
		return lines
	}

	rows := (len(items) + 1) / 2
	colA, colB := items[:rows], items[rows:]

	const minColWidth = 12
	leftWidth := widestCell(colA)
	if capWidth := width - minColWidth; leftWidth > capWidth && capWidth >= minColWidth {
		leftWidth = capWidth
	}
	rightWidth := max(width-leftWidth, minColWidth)

	lines := make([]string, rows)
	for i := 0; i < rows; i++ {
		left := padCell("  "+formatHelpBinding(colA[i]), leftWidth)
		right := ""
		if i < len(colB) {
			right = clipCell("  "+formatHelpBinding(colB[i]), rightWidth)
		}
		lines[i] = left + right
	}
	return lines
}

// widestCell returns the render width of items' widest "key help" cell.
func widestCell(items []KeyHint) int {
	w := 0
	for _, it := range items {
		if cw := lipgloss.Width("  " + formatHelpBinding(it)); cw > w {
			w = cw
		}
	}
	return w
}

// helpSectionLabelWidth aligns the vim and global ribbons on one label
// column ("global" is the widest label).
const helpSectionLabelWidth = 6

// renderHelpSectionLines renders one ribbon section — "label  key desc •
// key desc • ..." — packing items greedily onto as few lines as width
// allows, continuations indented under the ribbon column so every binding
// stays listed however narrow the panel.
func renderHelpSectionLines(label string, items []KeyHint, width int) []string {
	sep := " " + tui.IconBullet + " "
	prefix := padCell(helpOverlaySectionStyle.Render(label), helpSectionLabelWidth) + "  "
	indent := strings.Repeat(" ", helpSectionLabelWidth+2)
	budget := max(width-helpSectionLabelWidth-2, 12)

	var lines []string
	var cur strings.Builder
	curW := 0
	for _, it := range items {
		part := formatHelpBinding(it)
		partW := lipgloss.Width(part)
		switch {
		case curW == 0:
			cur.WriteString(part)
			curW = partW
		case curW+lipgloss.Width(sep)+partW <= budget:
			cur.WriteString(sep + part)
			curW += lipgloss.Width(sep) + partW
		default:
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(part)
			curW = partW
		}
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}

	for i, l := range lines {
		lead := indent
		if i == 0 {
			lead = prefix
		}
		lines[i] = clipCell(lead+l, width)
	}
	return lines
}

func formatHelpBinding(b KeyHint) string {
	return helpOverlayKeyStyle.Render(b.Key) + " " + helpStyle.Render(b.Help)
}

// clipCell clips s (already rendered, possibly styled) to at most width
// columns, ANSI-safely; s narrower than width is returned unchanged.
func clipCell(s string, width int) string {
	if lipgloss.Width(s) > width {
		return lipgloss.NewStyle().MaxWidth(width).Inline(true).Render(s)
	}
	return s
}

// padCell right-pads s to exactly width columns, clipping first if it's
// already too wide.
func padCell(s string, width int) string {
	s = clipCell(s, width)
	if w := lipgloss.Width(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

// clampWidth is a defensive backstop for width only — renderHelpOverlayBody
// already keeps row count within height by construction (trimming the
// screen section first, see its doc), so there's nothing to protect there;
// this just clips any row wider than width rather than trusting the
// border/padding math above never drifts.
func clampWidth(block string, width int) string {
	lines := strings.Split(block, "\n")
	for i, l := range lines {
		if lipgloss.Width(l) > width {
			lines[i] = lipgloss.NewStyle().MaxWidth(width).Inline(true).Render(l)
		}
	}
	return strings.Join(lines, "\n")
}
