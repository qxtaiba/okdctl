package tui

import (
	"bytes"
	"strings"
	"testing"
)

// TestBoxRespectsNoColorProfile is the regression guard for the highest-value
// finding in the CLI overhaul: box helpers historically emitted 24-bit
// truecolor unconditionally, so NO_COLOR and piped output kept every escape
// sequence. A non-TTY writer must yield a box with zero ANSI escapes.
func TestBoxRespectsNoColorProfile(t *testing.T) {
	SetColorProfileFor(&bytes.Buffer{}) // a plain buffer is never a TTY -> NoTTY
	t.Cleanup(func() { SetColorProfileFor(&bytes.Buffer{}) })

	if colorEnabled() {
		t.Fatal("colorEnabled should be false for a non-TTY writer")
	}

	out := BoxedSectionCompact("  phase ......... Running\n", "cluster status", DefaultBoxWidth)
	if strings.Contains(out, "\x1b[") {
		t.Errorf("boxed output leaked ANSI escapes under a no-color profile:\n%q", out)
	}
	if !strings.Contains(out, "CLUSTER STATUS") {
		t.Errorf("boxed output dropped its title:\n%s", out)
	}
}

func TestDownsampleStripsUnderNoColor(t *testing.T) {
	SetColorProfileFor(&bytes.Buffer{})
	t.Cleanup(func() { SetColorProfileFor(&bytes.Buffer{}) })

	styled := SuccessStyle.Render("ok")
	if got := downsample(styled); strings.Contains(got, "\x1b[") {
		t.Errorf("downsample left ANSI under no-color profile: %q", got)
	}
}
