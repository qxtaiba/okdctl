package tui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

// Regression guard: box helpers used to leak 24-bit escapes under NO_COLOR/pipes.
func TestBoxRespectsNoColorProfile(t *testing.T) {
	SetColorProfileFor(&bytes.Buffer{}) // a buffer is never a TTY
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
	if got := Downsample(styled); strings.Contains(got, "\x1b[") {
		t.Errorf("downsample left ANSI under no-color profile: %q", got)
	}
}

// Regression guard: colorprofile.Detect parses NO_COLOR with strconv.ParseBool,
// so a non-boolean value like "yes" left color enabled before detect() special-cased it.
func TestNoColorAnyValueForcesAscii(t *testing.T) {
	t.Setenv("NO_COLOR", "yes")
	t.Setenv("CLICOLOR_FORCE", "1")

	SetColorProfileFor(&bytes.Buffer{})
	t.Cleanup(func() { SetColorProfileFor(&bytes.Buffer{}) })

	if colorEnabled() {
		t.Fatal("colorEnabled should be false when NO_COLOR is set to any value")
	}
	if got := Downsample(SuccessStyle.Render("ok")); strings.Contains(got, "\x1b[") {
		t.Errorf("downsample left ANSI when NO_COLOR is set to any value: %q", got)
	}
}

func TestDisableColor(t *testing.T) {
	forced := colorprofile.TrueColor
	outputProfile.Store(&forced)
	t.Cleanup(func() { SetColorProfileFor(&bytes.Buffer{}) })

	DisableColor()

	if colorEnabled() {
		t.Fatal("colorEnabled should be false after DisableColor")
	}
	if got := Downsample(SuccessStyle.Render("ok")); strings.Contains(got, "\x1b[") {
		t.Errorf("downsample left ANSI after DisableColor: %q", got)
	}
}
