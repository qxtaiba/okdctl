package tui

import (
	"bytes"
	"io"
	"os"
	"sync/atomic"

	"github.com/charmbracelet/colorprofile"
)

// outputProfile is the color profile for rendered CLI surfaces (boxes,
// dotted lines, glyphs); box helpers used to leak 24-bit escapes under
// NO_COLOR/pipes, so this shares charm/log's TTY/NO_COLOR rulebook.
var outputProfile atomic.Pointer[colorprofile.Profile]

func init() {
	p := detect(os.Stdout)
	outputProfile.Store(&p)
}

// detect returns NoTTY when NO_COLOR is set to any value — colorprofile.Detect
// parses NO_COLOR as a bool, so a non-boolean value like "yes" would otherwise
// leak color — and delegates to colorprofile.Detect(w, os.Environ()) otherwise.
// NoTTY, not Ascii: colorprofile.Writer leaves bare "\x1b[m" reset markers
// around color-only spans at the Ascii profile (it only strips the color
// params, not the reset), while NoTTY takes the full ansi.Strip path.
func detect(w io.Writer) colorprofile.Profile {
	if os.Getenv("NO_COLOR") != "" {
		return colorprofile.NoTTY
	}
	return colorprofile.Detect(w, os.Environ())
}

// SetColorProfileFor re-detects the render color profile from w and the
// environment (TTY, NO_COLOR, CLICOLOR*). Call once at startup before any
// boxed print so redirected output/tests don't see the init-time os.Stdout
// snapshot.
func SetColorProfileFor(w io.Writer) {
	p := detect(w)
	outputProfile.Store(&p)
}

// DisableColor forces the render color profile to strip all ANSI for the
// rest of the process; see detect's doc for why NoTTY rather than Ascii.
func DisableColor() {
	p := colorprofile.NoTTY
	outputProfile.Store(&p)
}

func colorProfile() colorprofile.Profile {
	return *outputProfile.Load()
}

// colorEnabled reports whether the active profile emits any color.
func colorEnabled() bool {
	return colorProfile() > colorprofile.Ascii
}

// ColorEnabled reports whether the active color profile emits any color.
func ColorEnabled() bool {
	return colorEnabled()
}

// Downsample rewrites s so ANSI escapes match the active profile —
// unchanged under TrueColor, downgraded for ANSI/ANSI256, stripped
// otherwise. Boxed* helpers apply it internally; callers printing styled
// lines outside a box must call it themselves.
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
