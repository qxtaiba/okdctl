package tui

import (
	"image/color"
	"sync/atomic"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// Theme is one fully-resolved set of semantic color roles: every field is
// final for a given (color profile, background polarity, theme choice), so
// renderers read colors without consulting the environment again. Treat a
// Theme as immutable — inject it by pointer and never write through it.
type Theme struct {
	// Dark records the background polarity the theme resolved for.
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

// themeTable pairs the dark- and light-background values of one role.
type themeTable struct {
	dark, light string
}

// same returns a themeTable carrying v for both polarities.
func same(v string) themeTable {
	return themeTable{dark: v, light: v}
}

// roleTables is one theme's full token table, dual-polarity per role.
type roleTables struct {
	primary, primaryDim                themeTable
	success, warning, errorTone, info  themeTable
	text, textSoft, textDim, textFaint themeTable
	subtle, rule                       themeTable
	code, accent                       themeTable
}

// defaultTokens is the truecolor token table for the default theme; pairs
// are (dark-background, light-background).
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

// ansi256Tokens is the curated ANSI-256 pin table for the default theme:
// brand and status hues are hand-picked palette indices (not nearest-cube
// rounding), and the slate ladder pins to the grayscale ramp.
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

// ansi16BrightBlack is the shared muted tier at the 16-color profile.
const ansi16BrightBlack = "8"

// ansi16Tokens maps roles onto the 16-color palette so an ANSI-only
// terminal renders okdctl in the user's own theme colors: primary inherits
// magenta, accent/code cyan, and the text tiers the default foreground and
// bright black rather than a quantized slate. An empty value resolves to
// lipgloss.NoColor (the terminal's own foreground).
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

// High-contrast palette values, background-independent by design.
const (
	hcMagenta = "#FF00FF"
	hcCyan    = "#00FFFF"
	hcWhite   = "#FFFFFF"
	hcGray    = "#AAAAAA"
)

// highContrastTokens is the background-independent high-contrast table.
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

// ResolveTheme builds the Theme for one (color profile, background polarity,
// theme choice) triple; it is a pure function — resolution happens here,
// once, and the result never mutates.
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

// activeTheme is the process-wide resolved theme; swapped atomically so
// concurrent readers always see one consistent Theme.
var activeTheme atomic.Pointer[Theme]

// themeGeneration counts UseTheme installs; per-instance style caches pin
// the generation they were built at and rebuild once it moves.
var themeGeneration atomic.Uint64

// CurrentTheme returns a copy of the active resolved Theme.
func CurrentTheme() Theme {
	return *activeTheme.Load()
}

// ThemeGeneration reports how many times a theme has been installed, so a
// per-instance style cache built against one install rebuilds after the
// next instead of freezing its captured polarity.
func ThemeGeneration() uint64 {
	return themeGeneration.Load()
}

// UseTheme installs a copy of t as the active theme and rebuilds the derived
// style caches; the color getters are safe to read concurrently with it, the
// style vars are not — install before rendering starts.
func UseTheme(t Theme) { //nolint:gocritic // hugeParam: value keeps the installed copy immutable — the caller's Theme can't be written through afterwards
	activeTheme.Store(&t)
	themeGeneration.Add(1)
	rebuildStyles()
}

// resolveActiveTheme re-resolves the active theme from the current color
// profile and env-selected theme choice, keeping dark's polarity.
func resolveActiveTheme(dark bool) {
	choice := ThemeDefault
	if highContrastRequested() {
		choice = ThemeHighContrast
	}
	UseTheme(ResolveTheme(colorProfile(), dark, choice))
}

// Color getters: thin reads of the active resolved Theme, kept as the
// migration seam while surfaces move to an injected Theme wave by wave.

// ColorPrimary returns the active theme's brand hue.
func ColorPrimary() color.Color { return CurrentTheme().Primary }

// ColorPrimaryDim returns the active theme's brand-tinted border hue.
func ColorPrimaryDim() color.Color { return CurrentTheme().PrimaryDim }

// ColorSuccess returns the active theme's success hue.
func ColorSuccess() color.Color { return CurrentTheme().Success }

// ColorWarning returns the active theme's warning hue.
func ColorWarning() color.Color { return CurrentTheme().Warning }

// ColorError returns the active theme's error hue.
func ColorError() color.Color { return CurrentTheme().Error }

// ColorInfo returns the active theme's info hue.
func ColorInfo() color.Color { return CurrentTheme().Info }

// ColorText returns the active theme's brightest body-text tier.
func ColorText() color.Color { return CurrentTheme().Text }

// ColorTextSoft returns the active theme's secondary-value text tier.
func ColorTextSoft() color.Color { return CurrentTheme().TextSoft }

// ColorTextDim returns the active theme's muted text tier.
func ColorTextDim() color.Color { return CurrentTheme().TextDim }

// ColorTextFaint returns the active theme's faintest readable text tier.
func ColorTextFaint() color.Color { return CurrentTheme().TextFaint }

// ColorSubtle returns the active theme's sub-text structural tier (pending
// glyphs, soft borders).
func ColorSubtle() color.Color { return CurrentTheme().Subtle }

// ColorRule returns the active theme's separator/leader tier.
func ColorRule() color.Color { return CurrentTheme().Rule }

// ColorCode returns the active theme's inline-code hue.
func ColorCode() color.Color { return CurrentTheme().Code }

// ColorAccent returns the active theme's accent hue.
func ColorAccent() color.Color { return CurrentTheme().Accent }
