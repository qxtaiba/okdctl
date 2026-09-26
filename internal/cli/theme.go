package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/tui"
)

const (
	themeModeAll          = channelAll
	themeModeDark         = "dark"
	themeModeLight        = "light"
	themeModeHighContrast = "high-contrast"
	themeModeInherit      = "inherit"
)

var themePreviewMode string

var themeCmd = &cobra.Command{
	Use:   "theme",
	Short: "Inspect terminal theme styles",
}

var themePreviewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Preview the TUI color roles",
	Long: `Render okdctl's semantic TUI styles in dark, light, high-contrast, and
terminal-inherit modes. The preview is read-only and uses the current terminal
color profile; NO_COLOR and --no-color suppress ANSI styling.

Terminal-inherit uses the active resolved theme polarity. Interactive screens
can update that polarity from the terminal's background-color response; when
no response is available, okdctl keeps its dark-background default.`,
	Example: `  okdctl theme preview
  okdctl theme preview --mode light
  NO_COLOR=1 okdctl theme preview --mode inherit`,
	Args: cobra.NoArgs,
	RunE: runThemePreview,
}

func runThemePreview(cmd *cobra.Command, _ []string) error {
	profile := tui.OutputColorProfile()
	active := tui.CurrentTheme()
	modes := []string{themeModeDark, themeModeLight, themeModeHighContrast, themeModeInherit}
	if themePreviewMode != themeModeAll {
		modes = []string{themePreviewMode}
	}
	if !containsThemePreviewMode(themePreviewMode) {
		return &errtypes.UsageError{Msg: "mode must be all, dark, light, high-contrast, or inherit"}
	}

	var out strings.Builder
	fmt.Fprintf(&out, "okdctl theme preview · profile: %s\n", themeProfileName(profile))
	if profile == colorprofile.NoTTY {
		out.WriteString("ANSI styling: disabled\n")
	}
	for i, mode := range modes {
		var resolved tui.Theme
		switch mode {
		case themeModeDark:
			resolved = tui.ResolveTheme(profile, true, tui.ThemeDefault)
		case themeModeLight:
			resolved = tui.ResolveTheme(profile, false, tui.ThemeDefault)
		case themeModeHighContrast:
			resolved = tui.ResolveTheme(profile, active.Dark, tui.ThemeHighContrast)
		case themeModeInherit:
			resolved = active
		}
		if i > 0 {
			out.WriteByte('\n')
		}
		polarity := "light"
		if resolved.Dark {
			polarity = "dark"
		}
		label := mode
		if mode == themeModeInherit || mode == themeModeHighContrast {
			label += " · " + polarity + " background"
		}
		fmt.Fprintf(&out, "%s\n", label)
		out.WriteString(tui.RenderThemePreview(&resolved, profile))
	}
	_, err := fmt.Fprint(cmd.OutOrStdout(), out.String())
	return err
}

func containsThemePreviewMode(mode string) bool {
	switch mode {
	case themeModeAll, themeModeDark, themeModeLight, themeModeHighContrast, themeModeInherit:
		return true
	default:
		return false
	}
}

func themeProfileName(profile colorprofile.Profile) string {
	switch profile {
	case colorprofile.TrueColor:
		return "truecolor"
	case colorprofile.ANSI256:
		return "ansi256"
	case colorprofile.ANSI:
		return "ansi16"
	default:
		return "no-color"
	}
}

func init() {
	themePreviewCmd.Flags().StringVar(&themePreviewMode, "mode", themeModeAll, "preview one mode: all|dark|light|high-contrast|inherit")
	_ = themePreviewCmd.RegisterFlagCompletionFunc("mode", cobra.FixedCompletions(
		[]string{themeModeAll, themeModeDark, themeModeLight, themeModeHighContrast, themeModeInherit}, cobra.ShellCompDirectiveNoFileComp))
	themeCmd.AddCommand(themePreviewCmd)
	rootCmd.AddCommand(themeCmd)
}
