package tui

import (
	"image/color"
	"sync/atomic"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// Theme is one fully-resolved set of semantic color roles for a (profile,
// polarity, choice) triple. Treat it as immutable: inject by pointer, never
// write through it.
type Theme struct {
	Dark bool

	Primary    color.Color // brand hue: titles, active glyphs, highlights
	PrimaryDim color.Color // brand-tinted borders and frames

	Success color.Color
	Warning color.Color
	Error   color.Color
	Info    color.Color

	Text      color.Color // brightest body text
	TextSoft  color.Color // secondary values, one tier under Text
	TextDim   color.Color // muted labels and annotations
	TextFaint color.Color // faintest readable tier: nested keys, timestamps

	Subtle color.Color // structural, sub-text: pending glyphs, soft borders
	Rule   color.Color // separators, dot leaders

	Code   color.Color // inline code and credentials
	Accent color.Color // section headers, spinner, small emphasis
}

type themeTable struct {
	dark, light string
}

func same(v string) themeTable {
	return themeTable{dark: v, light: v}
}

type roleTables struct {
	primary, primaryDim                themeTable
	success, warning, errorTone, info  themeTable
	text, textSoft, textDim, textFaint themeTable
	subtle, rule                       themeTable
	code, accent                       themeTable
}

// defaultTokens pairs are (dark-background, light-background).
var defaultTokens = roleTables{
	primary:    themeTable{"#9333EA", "#7E22CE"},
	primaryDim: themeTable{"#6B21A8", "#C084FC"},
	success:    themeTable{"#22C55E", "#15803D"},
	warning:    themeTable{"#F59E0B", "#B45309"},
	errorTone:  themeTable{"#EF4444", "#DC2626"},
	info:       themeTable{"#3B82F6", "#2563EB"},
	text:       themeTable{"#F1F5F9", "#0F172A"},
	textSoft:   themeTable{"#CBD5E1", "#334155"},
	textDim:    themeTable{"#94A3B8", "#475569"},
	textFaint:  same("#64748B"),
	subtle:     themeTable{"#475569", "#94A3B8"},
	rule:       themeTable{"#334155", "#CBD5E1"},
	code:       themeTable{"#22D3EE", "#0E7490"},
	accent:     themeTable{"#06B6D4", "#0E7490"},
}

// ansi256Tokens hand-picks palette indices rather than nearest-cube rounding.
var ansi256Tokens = roleTables{
	primary:    themeTable{"135", "92"},
	primaryDim: themeTable{"55", "183"},
	success:    themeTable{"41", "28"},
	warning:    themeTable{"214", "130"},
	errorTone:  themeTable{"203", "160"},
	info:       themeTable{"69", "27"},
	text:       themeTable{"255", "235"},
	textSoft:   themeTable{"252", "237"},
	textDim:    themeTable{"248", "239"},
	textFaint:  same("245"),
	subtle:     themeTable{"239", "248"},
	rule:       themeTable{"237", "252"},
	code:       themeTable{"45", "30"},
	accent:     themeTable{"38", "30"},
}

const ansi16BrightBlack = "8"

// ansi16Tokens lets an ANSI-only terminal render okdctl in its own theme
// colors; an empty value resolves to lipgloss.NoColor.
var ansi16Tokens = roleTables{
	primary:    themeTable{"13", "5"},
	primaryDim: themeTable{"5", "13"},
	success:    themeTable{"10", "2"},
	warning:    themeTable{"11", "3"},
	errorTone:  themeTable{"9", "1"},
	info:       themeTable{"12", "4"},
	text:       same(""),
	textSoft:   same(""),
	textDim:    same(ansi16BrightBlack),
	textFaint:  same(ansi16BrightBlack),
	subtle:     same(ansi16BrightBlack),
	rule:       same(ansi16BrightBlack),
	code:       themeTable{"14", "6"},
	accent:     themeTable{"14", "6"},
}

const (
	hcMagenta = "#FF00FF"
	hcCyan    = "#00FFFF"
	hcWhite   = "#FFFFFF"
	hcGray    = "#AAAAAA"
)

var highContrastTokens = roleTables{
	primary:    same(hcMagenta),
	primaryDim: same(hcMagenta),
	success:    same("#00FF00"),
	warning:    same("#FFFF00"),
	errorTone:  same("#FF0000"),
	info:       same(hcCyan),
	text:       same(hcWhite),
	textSoft:   same(hcWhite),
	textDim:    same(hcGray),
	textFaint:  same(hcGray),
	subtle:     same(hcGray),
	rule:       same(hcGray),
	code:       same(hcCyan),
	accent:     same(hcCyan),
}

// ResolveTheme builds the Theme for one (profile, polarity, choice) triple.
func ResolveTheme(profile colorprofile.Profile, dark bool, choice ColorTheme) Theme {
	tokens := defaultTokens
	switch {
	case choice == ThemeHighContrast:
		tokens = highContrastTokens
	case profile == colorprofile.ANSI256:
		tokens = ansi256Tokens
	case profile == colorprofile.ANSI:
		tokens = ansi16Tokens
	}

	pick := func(t themeTable) color.Color {
		v := t.dark
		if !dark {
			v = t.light
		}
		if v == "" {
			return lipgloss.NoColor{}
		}
		return lipgloss.Color(v)
	}

	return Theme{
		Dark:       dark,
		Primary:    pick(tokens.primary),
		PrimaryDim: pick(tokens.primaryDim),
		Success:    pick(tokens.success),
		Warning:    pick(tokens.warning),
		Error:      pick(tokens.errorTone),
		Info:       pick(tokens.info),
		Text:       pick(tokens.text),
		TextSoft:   pick(tokens.textSoft),
		TextDim:    pick(tokens.textDim),
		TextFaint:  pick(tokens.textFaint),
		Subtle:     pick(tokens.subtle),
		Rule:       pick(tokens.rule),
		Code:       pick(tokens.code),
		Accent:     pick(tokens.accent),
	}
}

var activeTheme atomic.Pointer[Theme]

// CurrentTheme returns a copy of the active resolved Theme.
func CurrentTheme() Theme {
	return *activeTheme.Load()
}

// UseTheme installs a copy of t and rebuilds the derived style caches. The
// color getters are safe to read concurrently with it, the style vars are
// not, so install before rendering starts.
func UseTheme(t Theme) { //nolint:gocritic // hugeParam: value keeps the installed copy immutable — the caller's Theme can't be written through afterwards
	activeTheme.Store(&t)
	rebuildStyles()
}

func resolveActiveTheme(dark bool) {
	choice := ThemeDefault
	if highContrastRequested() {
		choice = ThemeHighContrast
	}
	UseTheme(ResolveTheme(colorProfile(), dark, choice))
}

// Color getters read the active resolved Theme.

// ColorPrimary returns the brand hue.
func ColorPrimary() color.Color { return CurrentTheme().Primary }

// ColorPrimaryDim returns the brand-tinted border hue.
func ColorPrimaryDim() color.Color { return CurrentTheme().PrimaryDim }

// ColorSuccess returns the success hue.
func ColorSuccess() color.Color { return CurrentTheme().Success }

// ColorWarning returns the warning hue.
func ColorWarning() color.Color { return CurrentTheme().Warning }

// ColorError returns the error hue.
func ColorError() color.Color { return CurrentTheme().Error }

// ColorInfo returns the info hue.
func ColorInfo() color.Color { return CurrentTheme().Info }

// ColorText returns the brightest body-text tier.
func ColorText() color.Color { return CurrentTheme().Text }

// ColorTextSoft returns the secondary-value text tier.
func ColorTextSoft() color.Color { return CurrentTheme().TextSoft }

// ColorTextDim returns the muted text tier.
func ColorTextDim() color.Color { return CurrentTheme().TextDim }

// ColorTextFaint returns the faintest readable text tier.
func ColorTextFaint() color.Color { return CurrentTheme().TextFaint }

// ColorSubtle returns the structural tier for pending glyphs and soft borders.
func ColorSubtle() color.Color { return CurrentTheme().Subtle }

// ColorRule returns the separator tier.
func ColorRule() color.Color { return CurrentTheme().Rule }

// ColorCode returns the inline-code hue.
func ColorCode() color.Color { return CurrentTheme().Code }

// ColorAccent returns the accent hue.
func ColorAccent() color.Color { return CurrentTheme().Accent }
