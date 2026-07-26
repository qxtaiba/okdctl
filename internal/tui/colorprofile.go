package tui

import (
	"bytes"
	"io"
	"os"
	"sync/atomic"

	"github.com/charmbracelet/colorprofile"
)

// outputProfile is the color profile applied to rendered CLI surfaces — boxes,
// dotted key/value lines, completion glyphs. charm/log already gates its level
// badges on TTY/NO_COLOR; the lipgloss box helpers historically did not, so a
// piped or NO_COLOR run leaked 24-bit escapes into files and dumb terminals.
// Detecting once here and downsampling every rendered string through the same
// colorprofile machinery charm/log uses gives both subsystems one rulebook.
var outputProfile atomic.Pointer[colorprofile.Profile]

func init() {
	p := colorprofile.Detect(os.Stdout, os.Environ())
	outputProfile.Store(&p)
}

// SetColorProfileFor re-detects the render color profile from w (the command's
// real stdout) and the process environment: TTY presence, NO_COLOR, CLICOLOR,
// CLICOLOR_FORCE. Call once during startup before any boxed surface prints so
// tests and redirected output observe the correct profile rather than the
// os.Stdout snapshot taken at init.
func SetColorProfileFor(w io.Writer) {
	p := colorprofile.Detect(w, os.Environ())
	outputProfile.Store(&p)
}

// colorProfile returns the active render color profile.
func colorProfile() colorprofile.Profile {
	return *outputProfile.Load()
}

// ColorEnabled reports whether the active profile emits any color.
func ColorEnabled() bool {
	return colorProfile() > colorprofile.Ascii
}

// Downsample rewrites s so its ANSI color escapes match the active output
// profile: returned unchanged under TrueColor, downgraded for ANSI/ANSI256,
// and stripped entirely when stdout is not a TTY or NO_COLOR is set. This is
// the single gate that makes lipgloss box output honor NO_COLOR and pipes.
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
