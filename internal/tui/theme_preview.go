package tui

import (
	"bytes"
	"fmt"
	"io"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// RenderThemePreview renders the CLI's semantic styles with the given output profile.
func RenderThemePreview(theme *Theme, profile colorprofile.Profile) string {
	rows := [][2]string{
		{"title", lipgloss.NewStyle().Bold(true).Foreground(theme.Primary).Render("OKDCTL cluster identity")},
		{"frame", lipgloss.NewStyle().Foreground(theme.PrimaryDim).Render("+-- cluster overview --+")},
		{"text", lipgloss.NewStyle().Foreground(theme.Text).Render("Proxmox production cluster")},
		{"text-soft", lipgloss.NewStyle().Foreground(theme.TextSoft).Render("api.lab.example")},
		{"text-dim", lipgloss.NewStyle().Foreground(theme.TextDim).Render("3 nodes · ready")},
		{"text-faint", lipgloss.NewStyle().Foreground(theme.TextFaint).Render("updated moments ago")},
		{"subtle", lipgloss.NewStyle().Foreground(theme.Subtle).Render(IconPending + " waiting")},
		{"rule", lipgloss.NewStyle().Foreground(theme.Rule).Render("------------------------------")},
		{"code", lipgloss.NewStyle().Foreground(theme.Code).Render("oc get nodes")},
		{"success", lipgloss.NewStyle().Bold(true).Foreground(theme.Success).Render(IconSuccess + " healthy")},
		{"warning", lipgloss.NewStyle().Bold(true).Foreground(theme.Warning).Render(IconWarning + " attention")},
		{"error", lipgloss.NewStyle().Bold(true).Foreground(theme.Error).Render(IconError + " unavailable")},
		// info pairs with no glyph anywhere in the app (logger.go renders a
		// "[INFO]" text badge instead), so the preview matches that: color only.
		{"info", lipgloss.NewStyle().Bold(true).Foreground(theme.Info).Render("update available")},
		{"accent", lipgloss.NewStyle().Bold(true).Foreground(theme.Accent).Render(IconActive + " active section")},
	}
	var b bytes.Buffer
	fmt.Fprintln(&b, "role       sample")
	fmt.Fprintln(&b, "────────── ─────────────────────────")
	for _, row := range rows {
		fmt.Fprintf(&b, "%-10s %s\n", row[0], row[1])
	}
	if profile == colorprofile.TrueColor {
		return b.String()
	}
	var rendered bytes.Buffer
	w := &colorprofile.Writer{Forward: &rendered, Profile: profile}
	_, _ = io.Copy(w, &b)
	return rendered.String()
}
