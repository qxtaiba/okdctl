// Package render builds the plain-text blocks the CLI prints.
package render

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
)

const tableIndent = 4

const defaultKeyColWidth = 45

// minWrapWidth floors the wrap width for Para and Bullet so a narrow box
// never collapses prose to an unreadable sliver.
const minWrapWidth = 12

// Builder accumulates aligned section/key-value lines for a boxed summary.
type Builder struct {
	b        strings.Builder
	keyWidth int
	kvWidth  int
}

// NewBuilder returns a Builder with the shared summary column widths.
func NewBuilder() *Builder {
	return NewBuilderWidth(tui.DefaultBoxWidth)
}

// NewBuilderWidth returns a Builder whose key and key/value columns fit
// inside a box of the given width.
func NewBuilderWidth(width int) *Builder {
	content := tui.BoxInnerWidth(width) - 2
	return &Builder{
		keyWidth: min(defaultKeyColWidth, content-24),
		kvWidth:  content,
	}
}

// ContentWidth reports the content width available for raw lines inside this Builder's box.
func (s *Builder) ContentWidth() int {
	return s.kvWidth
}

// Section writes a subsection label line.
func (s *Builder) Section(title string) {
	s.b.WriteString("  " + tui.SubsectionLabel(title) + "\n")
}

// kvOpts configures the private kv core shared by every KV* row.
type kvOpts struct {
	keyColWidth int // 0 = use the Builder's default keyWidth
	highlight   bool
	sub         bool
	link        bool // hyperlink the value to itself (the value is a URL)
}

// kv renders key/value through the tui dotted-line variant opts selects and
// writes the result, indenting every line (including any wrapped
// continuation) by the Builder's two-space margin.
func (s *Builder) kv(key, value string, opts kvOpts) {
	keyColWidth := opts.keyColWidth
	if keyColWidth <= 0 {
		keyColWidth = s.keyWidth
	}

	var rendered string
	switch {
	case opts.sub:
		rendered = tui.DottedKeyValueSubFull("  "+key, value, keyColWidth, s.kvWidth)
	case opts.highlight:
		rendered = tui.DottedKeyValueHighlightFull("  "+key, value, keyColWidth, s.kvWidth)
	case opts.link:
		rendered = tui.DottedKeyValueLinkFull("  "+key, value, keyColWidth, s.kvWidth)
	default:
		rendered = tui.DottedKeyValueFull("  "+key, value, keyColWidth, s.kvWidth)
	}
	s.writeIndented(rendered)
}

// writeIndented writes rendered — one or more newline-joined lines — each
// prefixed by the Builder's two-space margin.
func (s *Builder) writeIndented(rendered string) {
	for i, line := range strings.Split(rendered, "\n") {
		if i > 0 {
			s.b.WriteString("\n")
		}
		s.b.WriteString("  " + line)
	}
	s.b.WriteString("\n")
}

// writeWrapped writes text word-wrapped to fit the box, led by prefix on the
// first line and by indent spaces on every continuation; extra is how many
// columns prefix costs beyond the Builder's baseline two-space margin.
func (s *Builder) writeWrapped(prefix string, indent, extra int, text string) {
	width := max(s.kvWidth-extra, minWrapWidth)
	for i, line := range tui.WrapLines(text, width) {
		if i == 0 {
			s.b.WriteString(prefix + line + "\n")
		} else {
			s.b.WriteString(strings.Repeat(" ", indent) + line + "\n")
		}
	}
}

// KV writes a dotted key/value line.
func (s *Builder) KV(key, value string) {
	s.kv(key, value, kvOpts{})
}

// KVHighlight writes a dotted key/value line with a highlighted value.
func (s *Builder) KVHighlight(key, value string) {
	s.kv(key, value, kvOpts{highlight: true})
}

// KVWide writes a dotted key/value line with a 24-column key, leaving long command or URL values more room before they wrap.
func (s *Builder) KVWide(key, value string) {
	s.kv(key, value, kvOpts{keyColWidth: tui.DefaultKeyColWidth})
}

// KVLink writes KVWide's row with the URL value OSC 8-hyperlinked to itself,
// degrading to KVWide's plain text off-TTY and under NO_COLOR.
func (s *Builder) KVLink(key, url string) {
	s.kv(key, url, kvOpts{keyColWidth: tui.DefaultKeyColWidth, link: true})
}

// SubKV writes a nested dotted key/value line with a muted key and a key column narrowed by two.
func (s *Builder) SubKV(key, value string) {
	s.kv(key, value, kvOpts{keyColWidth: s.keyWidth - 2, sub: true})
}

// Note writes a labeled prose line with no dot leaders, wrapping the text under the value column.
func (s *Builder) Note(label, text string) {
	s.writeIndented(tui.KeyValueNote("  "+label, text, s.keyWidth, s.kvWidth))
}

// Para writes a word-wrapped paragraph indented two spaces.
func (s *Builder) Para(text string) {
	s.writeWrapped("  ", 2, 0, text)
}

// Bullet writes a bullet-prefixed, word-wrapped line with continuations aligned under the text.
func (s *Builder) Bullet(text string) {
	s.writeWrapped("    "+tui.IconBullet+" ", 6, 4, text)
}

// Table writes tui.Table's rendered lines indented four spaces, tightening
// opts.MaxColWidth (deriving a cap from the Builder's content width) when
// the caller's requested width would otherwise overflow the box.
func (s *Builder) Table(headers []string, rows [][]string, opts tui.TableOptions) {
	if colCap := s.fitTableCap(headers, rows, opts); colCap > 0 {
		opts.MaxColWidth = colCap
	}
	for _, line := range tui.Table(headers, rows, opts) {
		s.b.WriteString(strings.Repeat(" ", tableIndent) + tui.Downsample(line) + "\n")
	}
}

// fitTableCap returns the largest column-width cap, no wider than the
// caller's requested opts.MaxColWidth (0 meaning uncapped), that keeps the
// table's total rendered width within the Builder's content width; it
// returns 0 when the caller's request (or the table's natural, untruncated
// width) already fits, leaving opts untouched.
func (s *Builder) fitTableCap(headers []string, rows [][]string, opts tui.TableOptions) int {
	gap := opts.Gap
	if gap <= 0 {
		gap = 2
	}
	avail := s.kvWidth - tableIndent

	natural := make([]int, len(headers))
	for c, h := range headers {
		natural[c] = lipgloss.Width(h)
	}
	for _, row := range rows {
		for c := 0; c < len(headers) && c < len(row); c++ {
			natural[c] = max(natural[c], lipgloss.Width(row[c]))
		}
	}

	widthAt := func(capW int) int {
		total := gap * (len(headers) - 1)
		for _, w := range natural {
			if capW > 0 && w > capW {
				w = capW
			}
			total += w
		}
		return total
	}

	if widthAt(opts.MaxColWidth) <= avail {
		return 0
	}

	hi := 0
	for _, w := range natural {
		hi = max(hi, w)
	}
	if opts.MaxColWidth > 0 && opts.MaxColWidth < hi {
		hi = opts.MaxColWidth
	}
	for capW := hi; capW > 1; capW-- {
		if widthAt(capW) <= avail {
			return capW
		}
	}
	return 1
}

// Newline writes a blank spacer line between sections.
func (s *Builder) Newline() {
	s.b.WriteString("\n")
}

// WriteString appends raw text without key/section formatting.
func (s *Builder) WriteString(str string) {
	s.b.WriteString(str)
}

func (s *Builder) String() string {
	return s.b.String()
}

// DryRunStep is a single step entry for a dry-run plan listing.

// ValidationSummary renders a config validation result, listing each error with
// field context when present.
func ValidationSummary(result *config.ValidationResult) string {
	var sb strings.Builder

	if result.IsValid() {
		sb.WriteString(tui.CompletionSuccess("configuration is valid"))
	} else {
		sb.WriteString(tui.CompletionError(fmt.Sprintf("configuration invalid (%d errors)", len(result.Errors))))
		sb.WriteString("\n\n")
		for _, e := range result.Errors {
			sb.WriteString("  " + tui.ErrorStyle.Render(tui.IconError+" [fail]") + " ")
			if e.Field != "" {
				sb.WriteString(tui.MutedStyle.Render(e.Field + ": "))
			}
			sb.WriteString(e.Message + "\n")
		}
	}

	return tui.Downsample(sb.String())
}

// deployURLs returns cfg's cluster FQDN and its console and api URLs, the
// three strings the post-deploy summary and its recap both name.
