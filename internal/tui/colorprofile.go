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
	p := colorprofile.Detect(os.Stdout, os.Environ())
	outputProfile.Store(&p)
}

// SetColorProfileFor re-detects the render color profile from w and the
// environment (TTY, NO_COLOR, CLICOLOR*). Call once at startup before any
// boxed print so redirected output/tests don't see the init-time os.Stdout
// snapshot.
func SetColorProfileFor(w io.Writer) {
	p := colorprofile.Detect(w, os.Environ())
	outputProfile.Store(&p)
}

func colorProfile() colorprofile.Profile {
	return *outputProfile.Load()
}

// colorEnabled reports whether the active profile emits any color.
func colorEnabled() bool {
	return colorProfile() > colorprofile.Ascii
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
