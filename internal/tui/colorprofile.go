package tui

import (
	"bytes"
	"io"
	"os"
	"sync/atomic"

	"github.com/charmbracelet/colorprofile"
)

// outputProfile shares charm/log's TTY/NO_COLOR rulebook so boxes never
// leak 24-bit escapes into pipes.
var outputProfile atomic.Pointer[colorprofile.Profile]

func init() {
	p := detect(os.Stdout)
	outputProfile.Store(&p)
}

// detect returns NoTTY under any NO_COLOR value: colorprofile.Detect parses
// it as a bool, and Ascii would keep bare reset markers around color spans.
func detect(w io.Writer) colorprofile.Profile {
	if os.Getenv("NO_COLOR") != "" {
		return colorprofile.NoTTY
	}
	return colorprofile.Detect(w, os.Environ())
}

// SetColorProfileFor re-detects the render color profile from w and the
// environment. Call once at startup before any boxed print.
func SetColorProfileFor(w io.Writer) {
	p := detect(w)
	outputProfile.Store(&p)
	resolveActiveTheme(IsDarkBackground())
}

// DisableColor strips all ANSI from rendered output for the rest of the
// process.
func DisableColor() {
	p := colorprofile.NoTTY
	outputProfile.Store(&p)
	resolveActiveTheme(IsDarkBackground())
}

func colorProfile() colorprofile.Profile {
	return *outputProfile.Load()
}

func colorEnabled() bool {
	return colorProfile() > colorprofile.Ascii
}

// ColorEnabled reports whether the active color profile emits any color.
func ColorEnabled() bool {
	return colorEnabled()
}

// Hyperlink wraps text in an OSC 8 link to url, or returns it plain when the
// profile renders no escapes.
func Hyperlink(url, text string) string {
	if url == "" || !colorEnabled() {
		return text
	}
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// Downsample rewrites s's ANSI escapes to the active profile. Boxed* helpers
// apply it; callers printing styled lines outside a box must.
func Downsample(s string) string {
	p := colorProfile()
	if p == colorprofile.TrueColor {
		return s
	}
	var buf bytes.Buffer
	w := &colorprofile.Writer{Forward: &buf, Profile: p}
	_, _ = w.WriteString(s)
	return buf.String()
}
