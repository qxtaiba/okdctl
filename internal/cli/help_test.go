package cli

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/qxtaiba/okdctl/internal/tui"
)

func TestHelpFitsEightyColumnsAndIsPlain(t *testing.T) {
	tui.SetTerminalWidth(80)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })
	tui.SetColorProfileFor(&bytes.Buffer{})
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"node", "resize", "--help"})
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetArgs(nil)
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd.Execute(node resize --help) = %v", err)
	}
	got := out.String()

	if strings.Contains(got, "\x1b[") {
		t.Errorf("help output leaked ANSI escapes under no-color:\n%q", got)
	}
	if strings.Contains(got, "Usage:") {
		t.Errorf("help output still uses cobra's stock \"Usage:\" heading:\n%s", got)
	}
	if !strings.Contains(got, "\nflags\n") {
		t.Errorf("help output missing the \"flags\" section:\n%s", got)
	}
	if !strings.Contains(got, "\nglobal flags\n") {
		t.Errorf("help output missing the \"global flags\" section:\n%s", got)
	}

	lines := strings.Split(got, "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "okdctl › node resize") {
		t.Fatalf("first line = %q, want prefix %q", lines[0], "okdctl › node resize")
	}
	for i, line := range lines {
		if w := utf8.RuneCountInString(line); w > 80 {
			t.Errorf("line %d is %d columns, want <= 80: %q", i, w, line)
		}
	}
}

// Regression guard: SetHelpFunc's noColor gate (help.go) forces plain output
// for --help the way versionText does for --version (see
// TestVersionFlagRespectsNoColor); TestHelpFitsEightyColumnsAndIsPlain forces
// a plain profile before Execute, so it wouldn't catch that gate being
// deleted.
func TestHelpRespectsNoColor(t *testing.T) {
	// registered before t.Setenv so LIFO cleanup restores CLICOLOR_FORCE
	// first and only then re-detects the profile with a clean environment;
	// the reverse order leaves the package profile forced-colourful for
	// later tests
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })
	t.Setenv("CLICOLOR_FORCE", "1") // forces color even for a non-TTY writer

	tui.SetColorProfileFor(&bytes.Buffer{})

	if got := tui.Downsample(tui.SuccessStyle.Render("x")); !strings.Contains(got, "\x1b[") {
		t.Fatalf("test setup failed to force a colourful profile: %q", got)
	}

	var baseline bytes.Buffer
	rootCmd.SetOut(&baseline)
	rootCmd.SetArgs([]string{"node", "resize", "--help"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd.Execute(node resize --help) = %v", err)
	}
	if !strings.Contains(baseline.String(), "\x1b[") {
		t.Fatalf("test setup failed to force colourful help output: %q", baseline.String())
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"--no-color", "node", "resize", "--help"})
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetArgs(nil)
		noColor = false
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd.Execute(--no-color node resize --help) = %v", err)
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Errorf("node resize --help leaked ANSI under --no-color: %q", out.String())
	}
}

func TestRootHelpHeader(t *testing.T) {
	tui.SetColorProfileFor(&bytes.Buffer{})
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })

	got := helpHeader(rootCmd)
	want := "okdctl  " + rootCmd.Short
	if got != want {
		t.Fatalf("helpHeader(rootCmd) = %q, want %q", got, want)
	}
}

func TestNoCobraGroups(t *testing.T) {
	if got := len(rootCmd.Groups()); got != 0 {
		t.Fatalf("rootCmd.Groups() = %d, want 0 (the usage template omits the group branch)", got)
	}
}

// Pins that the usage template's dropped "Additional help topics" branch
// (cobra's HasHelpSubCommands) is currently a no-op: if a future command ever
// registers as an additional-help-topic command, it would silently vanish
// from --help under this template.
func TestNoAdditionalHelpTopicCommands(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.IsAdditionalHelpTopicCommand() {
			t.Errorf("%s is an additional-help-topic command; the usage template drops that section and would hide it", c.CommandPath())
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)
}

func TestLongestFlagUsageUnderCeiling(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		check := func(f *pflag.Flag) {
			if n := utf8.RuneCountInString(f.Usage); n >= 160 {
				t.Errorf("%s --%s: Usage is %d chars, want < 160: %q", c.CommandPath(), f.Name, n, f.Usage)
			}
		}
		c.Flags().VisitAll(check)
		c.PersistentFlags().VisitAll(check)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)
}
