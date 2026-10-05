package steps

import (
	"errors"
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui"
)

const (
	opsRefreshInterval     = 30 * time.Second
	opsLatencyHistoryLimit = 12
)

var opsLatencyGlyphs = []rune(tui.IconLatencySparkline)

type opsLatencySample struct {
	duration  time.Duration
	available bool
}

type opsSnapshot struct {
	status           *okd.ClusterStatus
	updated          time.Time
	latency          time.Duration
	latencyAvailable bool
	latencyHistory   []opsLatencySample
}

type opsSnapshotMsg struct {
	generation       uint64
	status           *okd.ClusterStatus
	err              error
	latency          time.Duration
	latencyAvailable bool
}

type opsRefreshMsg struct {
	generation uint64
}

func (s *WelcomeStep) probeOps(generation uint64) tea.Cmd {
	if s.opsLoading || s.opsSource == nil || s.opsCtx == nil {
		return nil
	}
	s.opsLoading = true
	ctx := s.opsCtx
	source := s.opsSource
	return func() tea.Msg {
		started := time.Now()
		status, err := source.ClusterStatus(ctx)
		latency := time.Since(started)
		if err == nil && status == nil {
			err = errEmptyOpsSnapshot
		}
		latencyAvailable := err == nil && status.APIAvailable && status.APIReachable
		return opsSnapshotMsg{
			generation: generation, status: status, err: err,
			latency: latency, latencyAvailable: latencyAvailable,
		}
	}
}

func appendOpsLatency(history []opsLatencySample, sample opsLatencySample) []opsLatencySample {
	history = append(history, sample)
	if len(history) > opsLatencyHistoryLimit {
		history = append([]opsLatencySample(nil), history[len(history)-opsLatencyHistoryLimit:]...)
	}
	return history
}

func (s *WelcomeStep) scheduleOpsRefresh(generation uint64) tea.Cmd {
	ctx := s.opsCtx
	return func() tea.Msg {
		timer := time.NewTimer(opsRefreshInterval)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			return opsRefreshMsg{generation: generation}
		}
	}
}

func renderOpsDashboard(status *opsSnapshot, loading bool, err error, width, height int) string {
	if width < 1 {
		return ""
	}
	if width < 112 || height < 30 {
		return renderOpsCompact(status, loading, err, width, height)
	}
	return renderOpsWide(status, loading, err, width, height)
}

func renderOpsWide(snapshot *opsSnapshot, loading bool, err error, width, height int) string {
	phase, api, nodes, operators, addonSummary := opsValues(snapshot)
	if snapshot == nil {
		phase, api, nodes, operators = "—", statusWaiting, statusWaiting, statusWaiting
	}
	if loading && snapshot == nil {
		api = "probing"
	}
	updated := "refreshing"
	if snapshot != nil {
		updated = "updated " + snapshot.updated.UTC().Format("15:04 UTC")
	}
	if err != nil && snapshot == nil {
		updated = "snapshot unavailable · retry scheduled"
	} else if err != nil {
		updated = "refresh failed · showing last snapshot"
	}

	gap := 2
	cardWidth := max(18, (width-gap*3)/4)
	tiles := []string{
		opsMetricCard("CLUSTER PHASE", phase, "lifecycle", cardWidth, tui.ColorPrimary()),
		opsMetricCard("API", api, renderOpsLatency(snapshot), cardWidth, opsHealthColor(snapshot, "api")),
		opsMetricCard("NODES", nodes, nodeBreakdown(snapshot), cardWidth, opsHealthColor(snapshot, "nodes")),
		opsMetricCard("OPERATORS", operators, addonSummary, cardWidth, opsHealthColor(snapshot, "operators")),
	}
	metrics := lipgloss.JoinHorizontal(lipgloss.Top, tiles[0], strings.Repeat(" ", gap), tiles[1], strings.Repeat(" ", gap), tiles[2], strings.Repeat(" ", gap), tiles[3])

	metrics += "\n\n" + renderNodeCard(snapshot, width, height)
	metrics += "\n\n" + renderAddonCard(snapshot, width)

	header := lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true).Render("CLUSTER OPERATIONS") + "   " +
		lipgloss.NewStyle().Foreground(tui.ColorTextDim()).Render(updated)
	if height >= 44 {
		if run := renderOpsLastRun(snapshot); run != "" {
			header += "\n" + run
		}
	}
	return header + "\n\n" + metrics
}

func opsMetricCard(title, value, detail string, width int, accent color.Color) string {
	valueStyle := lipgloss.NewStyle().Foreground(accent).Bold(true)
	detailStyle := lipgloss.NewStyle().Foreground(tui.ColorTextDim())
	body := valueStyle.Render(value) + "\n" + detailStyle.Render(detail)
	return tui.Card(title, body, width, accent)
}

func renderNodeCard(snapshot *opsSnapshot, width, height int) string {
	rows := []string{}
	switch {
	case snapshot == nil || !snapshot.status.NodesAvailable:
		rows = append(rows, "Node inventory unavailable")
	case len(snapshot.status.Nodes) == 0:
		rows = append(rows, "No nodes reported")
	default:
		counts := nodeCounts(snapshot.status.Nodes)
		ready := 0
		for _, node := range snapshot.status.Nodes {
			if node.Ready {
				ready++
			}
		}
		rows = append(rows, fmt.Sprintf("Ready %d/%d · %d master · %d worker", ready, counts.total, counts.masters, counts.workers))
		maxRows := max(2, height/3)
		for i, node := range snapshot.status.Nodes {
			if i >= maxRows {
				rows = append(rows, fmt.Sprintf("… %d more nodes", len(snapshot.status.Nodes)-i))
				break
			}
			mark, state, style := tui.IconError, statusNotReady, tui.ColorError()
			if node.Ready {
				mark, state, style = tui.IconSuccess, statusReady, tui.ColorSuccess()
			}
			role := string(node.Role)
			if role == "" || node.Role == nodetypes.RoleUnknown {
				role = "node"
			}
			rows = append(rows, lipgloss.NewStyle().Foreground(style).Render(mark)+"  "+
				lipgloss.NewStyle().Foreground(tui.ColorText()).Bold(true).Render(node.Name)+
				"   "+lipgloss.NewStyle().Foreground(tui.ColorTextDim()).Render(role+" · "+state))
		}
	}
	return tui.Card("NODE FLEET", strings.Join(rows, "\n"), width, tui.ColorAccent())
}

func renderAddonCard(snapshot *opsSnapshot, width int) string {
	rows := []string{}
	switch {
	case snapshot == nil || !snapshot.status.OperatorsAvailable:
		rows = append(rows, "Operator health unavailable")
	case len(snapshot.status.Addons) == 0:
		rows = append(rows, "No enabled add-ons reported")
	default:
		rows = append(rows, fmt.Sprintf("%d degraded operators · %s", snapshot.status.DegradedOperators, addonHealthSummary(snapshot)))
		for _, addon := range snapshot.status.Addons {
			mark, state, style := tui.IconError, addon.Label(), tui.ColorWarning()
			if addon.Healthy {
				mark, style = tui.IconSuccess, tui.ColorSuccess()
			}
			rows = append(rows, lipgloss.NewStyle().Foreground(style).Render(mark)+"  "+addon.Name+
				"   "+lipgloss.NewStyle().Foreground(tui.ColorTextDim()).Render(state))
		}
		if snapshot.status.DegradedOperators == 0 {
			rows = append(rows, lipgloss.NewStyle().Foreground(tui.ColorSuccess()).Render("All operators healthy"))
		}
	}
	return tui.Card("ADD-ONS & OPERATORS", strings.Join(rows, "\n"), width, tui.ColorInfo())
}

func renderOpsCompact(snapshot *opsSnapshot, loading bool, err error, width, height int) string {
	phase, api, nodes, operators, _ := opsValues(snapshot)
	if snapshot == nil {
		phase, api, nodes, operators = statusWaiting, statusWaiting, statusWaiting, statusWaiting
		if loading {
			api = "probing"
		} else if err != nil {
			api = statusUnavailable
		}
	}
	updated := ""
	if err != nil {
		updated = " · refresh failed"
	} else if snapshot != nil {
		updated = " · " + snapshot.updated.UTC().Format("15:04")
	}
	rows := []string{
		lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true).Render("CLUSTER OPERATIONS") + " · " + phase + updated,
		"API " + api + " · " + renderOpsLatencyCompact(snapshot),
		"NODES " + nodes + " · OPERATORS " + operators,
	}
	if width >= 100 && height >= 44 {
		if run := renderOpsLastRun(snapshot); run != "" {
			rows = append(rows, fitOpsLine(width, run))
		}
	}
	if snapshot != nil && snapshot.status.NodesAvailable && len(snapshot.status.Nodes) > 0 {
		for _, node := range snapshot.status.Nodes {
			mark, state := tui.IconError, "not ready"
			if node.Ready {
				mark, state = tui.IconSuccess, "ready"
			}
			role := string(node.Role)
			if role == "" || node.Role == nodetypes.RoleUnknown {
				role = "node"
			}
			rows = append(rows, fitOpsLine(width, mark+" "+node.Name+" · "+role+" · "+state))
		}
	} else {
		rows = append(rows, "Node detail unavailable")
	}
	if snapshot != nil && snapshot.status.OperatorsAvailable {
		for _, addon := range snapshot.status.Addons {
			rows = append(rows, fitOpsLine(width, "ADD-ON  "+addon.Name+" · "+addon.Label()))
		}
	} else {
		rows = append(rows, "Operator detail unavailable")
	}
	return strings.Join(rows, "\n")
}

func renderOpsLastRun(snapshot *opsSnapshot) string {
	if snapshot == nil || snapshot.status.LastDeployRunID == "" || snapshot.status.LastDeployAt.IsZero() {
		return ""
	}
	return "LAST RUN " + snapshot.status.LastDeployCluster + " · " + snapshot.status.LastDeployRunID +
		" · " + formatOpsAge(snapshot.updated, snapshot.status.LastDeployAt)
}

func formatOpsAge(now, at time.Time) string {
	age := now.Sub(at)
	switch {
	case age < 0:
		return "time unknown"
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return strconv.Itoa(int(age/time.Minute)) + "m ago"
	case age < 24*time.Hour:
		return strconv.Itoa(int(age/time.Hour)) + "h ago"
	default:
		return strconv.Itoa(int(age/(24*time.Hour))) + "d ago"
	}
}

func renderOpsLatencyCompact(snapshot *opsSnapshot) string {
	if snapshot == nil || len(snapshot.latencyHistory) == 0 {
		return "RTT unavailable"
	}
	var chart strings.Builder
	for _, sample := range snapshot.latencyHistory {
		if !sample.available {
			chart.WriteString(tui.IconLevelInfo)
			continue
		}
		chart.WriteRune(opsLatencyGlyph(sample.duration, snapshot.latencyHistory))
	}
	if !snapshot.latencyAvailable {
		return "RTT unavailable " + chart.String()
	}
	return "RTT " + formatOpsLatency(snapshot.latency) + " " + chart.String()
}

func formatOpsLatency(latency time.Duration) string {
	if latency < time.Millisecond {
		return "<1ms"
	}
	return fmt.Sprintf("%dms", latency.Milliseconds())
}

func renderOpsActionRows(s *WelcomeStep, width, height int) string {
	descriptions := map[HubVerb]string{
		HubVerbDeploy:        "apply the saved cluster configuration",
		HubVerbEditConfig:    "change cluster settings",
		HubVerbManageNodes:   "add or remove cluster nodes",
		HubVerbClusterStatus: "inspect live health details",
		HubVerbDestroy:       "remove cluster resources",
	}
	lines := strings.Split(s.nav.ViewPointer(), "\n")
	rows := make([]string, 0, len(lines)*2)
	for i, entry := range s.entries {
		if i >= len(lines) {
			break
		}
		if width >= 160 && height >= 44 {
			rows = append(rows, lines[i], lipgloss.NewStyle().Foreground(tui.ColorTextDim()).Render("      "+descriptions[entry.verb]))
			continue
		}
		rows = append(rows, lines[i]+"   "+lipgloss.NewStyle().Foreground(tui.ColorTextDim()).Render(descriptions[entry.verb]))
	}
	return strings.Join(rows, "\n")
}

// renderOpsLatency renders the API tile's RTT readout, which the wide
// dashboard must keep inside the narrowest tile it ever draws (24 inner
// columns at the 112-column compact cutoff) even with a full
// opsLatencyHistoryLimit-sample history, so it stays short and fixed-shape
// rather than growing with the label.
func renderOpsLatency(snapshot *opsSnapshot) string {
	if snapshot == nil || len(snapshot.latencyHistory) == 0 {
		return "RTT unavailable"
	}
	current := "n/a"
	if snapshot.latencyAvailable {
		current = formatOpsLatency(snapshot.latency)
	}
	var chart strings.Builder
	chart.WriteString("RTT ")
	chart.WriteString(current)
	chart.WriteString(" ")
	for _, sample := range snapshot.latencyHistory {
		if !sample.available {
			chart.WriteString(tui.IconLevelInfo)
			continue
		}
		chart.WriteRune(opsLatencyGlyph(sample.duration, snapshot.latencyHistory))
	}
	return chart.String()
}

func opsLatencyGlyph(duration time.Duration, samples []opsLatencySample) rune {
	var minValue, maxValue time.Duration
	first := true
	for _, sample := range samples {
		if !sample.available {
			continue
		}
		if first || sample.duration < minValue {
			minValue = sample.duration
		}
		if first || sample.duration > maxValue {
			maxValue = sample.duration
		}
		first = false
	}
	if first || maxValue == minValue {
		return opsLatencyGlyphs[len(opsLatencyGlyphs)/2]
	}
	level := int((duration - minValue) * time.Duration(len(opsLatencyGlyphs)-1) / (maxValue - minValue))
	return opsLatencyGlyphs[max(0, min(level, len(opsLatencyGlyphs)-1))]
}

// fitOpsLine rune-safely truncates line to width columns with a visible
// "…" so a status word past the fit point is elided, never silently
// wrapped away.
func fitOpsLine(width int, line string) string {
	return tui.Truncate(line, width)
}

func opsValues(snapshot *opsSnapshot) (phase, api, nodes, operators, addons string) {
	if snapshot == nil {
		return "unknown", statusUnavailable, statusUnavailable, statusUnavailable, "add-ons unavailable"
	}
	st := snapshot.status
	phase = string(st.Phase)
	if phase == "" || st.Phase == okd.PhaseUnknown {
		phase = "unknown"
	}
	api = statusUnavailable
	if st.APIAvailable {
		api = "unreachable"
		if st.APIReachable {
			api = "reachable"
		}
	}
	nodes = statusUnavailable
	if st.NodesAvailable {
		ready := 0
		for _, node := range st.Nodes {
			if node.Ready {
				ready++
			}
		}
		nodes = strconv.Itoa(ready) + "/" + strconv.Itoa(len(st.Nodes)) + " ready"
	}
	operators = statusUnavailable
	if st.OperatorsAvailable {
		operators = strconv.Itoa(st.DegradedOperators) + " degraded"
	}
	addons = "add-ons unavailable"
	if st.OperatorsAvailable {
		healthy := 0
		for _, addon := range st.Addons {
			if addon.Healthy {
				healthy++
			}
		}
		addons = strconv.Itoa(healthy) + "/" + strconv.Itoa(len(st.Addons)) + " add-ons healthy"
	}
	return phase, api, nodes, operators, addons
}

func nodeBreakdown(snapshot *opsSnapshot) string {
	if snapshot == nil || !snapshot.status.NodesAvailable {
		return "inventory unavailable"
	}
	counts := nodeCounts(snapshot.status.Nodes)
	return fmt.Sprintf("%d master · %d worker", counts.masters, counts.workers)
}

func addonHealthSummary(snapshot *opsSnapshot) string {
	healthy := 0
	for _, addon := range snapshot.status.Addons {
		if addon.Healthy {
			healthy++
		}
	}
	return fmt.Sprintf("%d/%d enabled add-ons healthy", healthy, len(snapshot.status.Addons))
}

func opsHealthColor(snapshot *opsSnapshot, section string) color.Color {
	if snapshot == nil {
		return tui.ColorTextDim()
	}
	st := snapshot.status
	switch section {
	case "api":
		if st.APIAvailable && st.APIReachable {
			return tui.ColorSuccess()
		}
		return tui.ColorError()
	case "nodes":
		if st.NodesAvailable {
			for _, node := range st.Nodes {
				if !node.Ready {
					return tui.ColorWarning()
				}
			}
			return tui.ColorSuccess()
		}
	case "operators":
		if st.OperatorsAvailable {
			if st.DegradedOperators == 0 {
				return tui.ColorSuccess()
			}
			return tui.ColorWarning()
		}
	}
	return tui.ColorTextDim()
}

var errEmptyOpsSnapshot = errors.New("read operations snapshot: empty result")
