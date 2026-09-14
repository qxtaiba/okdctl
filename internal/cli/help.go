package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// helpWrapCeiling caps flag-usage wrapping at 80 columns even on a wider terminal.
const helpWrapCeiling = 80

// installHelp wires okdctl's branded help/usage templates and their template
// funcs onto root; call once from root's init so every subcommand inherits
// them (cobra templates walk up to the nearest ancestor that set one).
func installHelp(root *cobra.Command) {
	// registered as template funcs rather than baked directly into the
	// templates so each renders after flags parse, colour gates included —
	// versionText carried this same reasoning on its own before it moved here
	cobra.AddTemplateFunc("header", helpHeader)
	cobra.AddTemplateFunc("section", section)
	cobra.AddTemplateFunc("muted", muted)
	cobra.AddTemplateFunc("helpWrapCols", helpWrapCols)
	cobra.AddTemplateFunc("versionText", versionText)

	root.SetVersionTemplate("{{versionText}}")

	root.SetHelpTemplate(`{{header .}}
{{with (or .Long .Short)}}
{{. | trimTrailingWhitespaces}}
{{end}}{{if or .Runnable .HasSubCommands}}
{{.UsageString}}{{end}}
`)

	root.SetUsageTemplate(`{{section "usage"}}{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

{{section "aliases"}}
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

{{section "examples"}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

{{section "commands"}}{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{section "flags"}}
{{.LocalFlags.FlagUsagesWrapped helpWrapCols | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

{{section "global flags"}}
{{.InheritedFlags.FlagUsagesWrapped helpWrapCols | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}

{{muted (printf "use \"%s [command] --help\" for more information about a command" .CommandPath)}}{{end}}
`)

	prev := root.HelpFunc()
	root.SetHelpFunc(func(c *cobra.Command, args []string) {
		if noColor {
			tui.DisableColor()
		}
		prev(c, args)
	})
}

// helpHeader renders c's breadcrumb-style help header, e.g. "okdctl › node
// resize  <short>" (root: "okdctl  <short>"), downsampled to the active
// color profile.
func helpHeader(c *cobra.Command) string {
	root := c.Root().Name()
	name := root
	if c.HasParent() {
		name += " › " + strings.TrimPrefix(c.CommandPath(), root+" ")
	}
	return tui.Downsample(tui.TitleStyle.Render(name) + "  " + c.Short)
}

func section(s string) string {
	return tui.Downsample(tui.TitleStyle.Render(s))
}

func muted(s string) string {
	return tui.Downsample(tui.MutedStyle.Render(s))
}

func helpWrapCols() int {
	return min(helpWrapCeiling, tui.TerminalWidth())
}
