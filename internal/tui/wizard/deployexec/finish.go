package deployexec

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

const (
	twoColumnMinWidth     = 92
	columnGap             = 4
	wordmarkMargin        = 2
	finishAnimationFrames = 5
)

const finishWord = "DEPLOYED"

func finishWordmark(col int, frame uint64) string {
	if !tui.ColorEnabled() || col < tui.WordmarkWidth(finishWord, 1)+wordmarkMargin {
		return lipgloss.NewStyle().Foreground(tui.ColorSuccess()).Bold(true).
			Render(strings.Join(strings.Split(finishWord, ""), " "))
	}
	return tui.Wordmark(finishWord, finishGradient(frame), 1)
}

func finishGradient(frame uint64) []color.Color {
	gradient := tui.SuccessGradient(len(finishWord))
	if frame > 0 {
		highlight := int((frame - 1) * uint64(len(gradient)-1) / uint64(finishAnimationFrames-1)) //nolint:gosec // the frame and gradient lengths are bounded by the finish animation.
		gradient[highlight] = tui.Lighten(gradient[highlight], 0.65)
	}
	return gradient
}

func finishHeadline(st *State, sty *wizard.ExecStyles, col int) string {
	left := st.Cfg.Cluster.Name + "." + st.Cfg.Cluster.Domain
	right := fmtDur(st.Elapsed)
	if st.RunID != "" {
		right = st.RunID + " · " + right
	}
	return justify(sty.Bold.Render(left), sty.Dim.Render(right), col)
}

func finishSummary(st *State, sty *wizard.ExecStyles, col int) string {
	f := render.NewPostDeployFacts(st.Cfg, st.Summary, st.Steps)
	twoColumn := col >= twoColumnMinWidth
	width := col
	if twoColumn {
		width = (col - columnGap) / 2
	}

	blocks := [][]string{
		factBlock(sty, "access", f.Access, width),
		factBlock(sty, "dns records", f.DNS, width),
		factBlock(sty, "credentials", f.Credentials, width),
		factBlock(sty, "status", f.Status, width),
		factBlock(sty, "steps", f.Steps, width),
		commandBlock(sty, "quick start", f.QuickStart),
	}
	if !twoColumn {
		return section(blocks...)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(width+columnGap).Render(section(blocks[:3]...)),
		section(blocks[3:]...))
}

func factBlock(sty *wizard.ExecStyles, title string, rows []tui.FactRow, width int) []string {
	if len(rows) == 0 {
		return nil
	}
	lines := tui.RenderFacts(rows, &tui.FactLayout{
		Leader: tui.FactLeaderDots, KeyWidth: factKeyCol, TotalWidth: width, Styles: tui.DefaultFactStyles(),
	})
	return append([]string{sty.Dim.Render(strings.ToUpper(title))}, lines...)
}

// commandBlock keeps copy-paste commands on one line so wrapping cannot
// change their meaning.
func commandBlock(sty *wizard.ExecStyles, title string, cmds []string) []string {
	if len(cmds) == 0 {
		return nil
	}
	lines := []string{sty.Dim.Render(strings.ToUpper(title))}
	for _, c := range cmds {
		lines = append(lines, "  "+tui.CodeInlineStyle.Render(c))
	}
	return lines
}

// ocLoginCmd omits the password because oc reads it from the kubeadmin file.
func ocLoginCmd(st *State) string {
	if st.Cfg == nil {
		return ""
	}
	return fmt.Sprintf("oc login -u kubeadmin https://api.%s.%s:6443",
		st.Cfg.Cluster.Name, st.Cfg.Cluster.Domain)
}

// finishBindings omits actions without providers so the ribbon matches the screen.
func finishBindings(hooks *Hooks) []wizard.KeyBinding {
	var keys []wizard.KeyBinding
	if hooks.Finish != nil && hooks.Finish.ClusterStatus != nil {
		keys = append(keys, wizard.KeyBinding{Key: string(rune(keyClusterStatus)), Help: "cluster status"})
	}
	if hooks.Finish != nil && hooks.Finish.ManageNodes != nil {
		keys = append(keys, wizard.KeyBinding{Key: string(rune(keyManageNodes)), Help: "manage nodes"})
	}
	if hooks.Finish != nil && hooks.Finish.OpenConsole != nil {
		keys = append(keys, wizard.KeyBinding{Key: string(rune(keyOpenConsole)), Help: "open console"})
	}
	return append(keys, wizard.KeyBinding{Key: string(rune(keyCopy)), Help: "copy the oc login command"})
}

func finishVerbs(hooks *Hooks, sty *wizard.ExecStyles, col int, copied bool) []string {
	var rows []tui.FactRow
	for _, b := range finishBindings(hooks) {
		rows = append(rows, tui.FactRow{Key: b.Key, Value: b.Help})
	}
	rows = append(rows, tui.FactRow{Key: "enter", Value: "exit"})
	lines := tui.RenderFacts(rows, &tui.FactLayout{
		Leader: tui.FactLeaderPad, KeyWidth: 8, TotalWidth: col,
		Styles: tui.FactStyles{Key: sty.Active, Value: sty.Dim},
	})
	if copied {
		lines = append(lines, sty.Warn.Render("        sent the oc login command to the clipboard (OSC 52)"))
	}
	return append([]string{sty.Dim.Render("NEXT")}, lines...)
}
