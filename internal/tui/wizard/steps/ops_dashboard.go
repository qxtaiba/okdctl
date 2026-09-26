package steps

import (
	"errors"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/tui"
)

const opsRefreshInterval = 30 * time.Second

type opsSnapshot struct {
	status  *okd.ClusterStatus
	updated time.Time
}

type opsSnapshotMsg struct {
	generation uint64
	status     *okd.ClusterStatus
	err        error
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
		status, err := source.ClusterStatus(ctx)
		if err == nil && status == nil {
			err = errEmptyOpsSnapshot
		}
		return opsSnapshotMsg{generation: generation, status: status, err: err}
	}
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

func renderOpsDashboard(status *opsSnapshot, loading bool, err error, width int) string {
	rows := make([]string, 0, 4)
	switch {
	case status == nil && loading:
		rows = append(rows, "Reading cluster status…")
	case status == nil && err != nil:
		rows = append(rows, "Snapshot unavailable · retry scheduled")
	case status == nil:
		rows = append(rows, "No live snapshot available")
	default:
		if err != nil {
			rows = append(rows, "Refresh failed · showing last snapshot")
		}
		st := status.status
		rows = append(rows, "API "+opsAPIStatus(st))
		if st.NodesAvailable {
			ready := 0
			counts := nodeCounts(st.Nodes)
			for _, node := range st.Nodes {
				if node.Ready {
					ready++
				}
			}
			rows = append(rows, "Nodes "+strconv.Itoa(ready)+"/"+strconv.Itoa(counts.total)+" ready · "+strconv.Itoa(counts.masters)+" master · "+strconv.Itoa(counts.workers)+" worker")
		} else {
			rows = append(rows, "Nodes unavailable")
		}
		if st.OperatorsAvailable {
			rows = append(rows, "Operators "+strconv.Itoa(st.DegradedOperators)+" degraded")
		} else {
			rows = append(rows, "Operators unavailable")
		}
		if st.LastDeployRunID != "" && !st.LastDeployAt.IsZero() {
			rows = append(rows, "Last recorded run "+st.LastDeployCluster+" · "+st.LastDeployRunID+" · "+formatOpsAge(status.updated, st.LastDeployAt))
		} else {
			rows = append(rows, "Last recorded run unavailable")
		}
		rows = append(rows, "Updated "+status.updated.UTC().Format("15:04:05 UTC"), "s open full cluster status")
	}
	return tui.Card("LIVE OPERATIONS", joinOpsRows(rows), width, tui.ColorPrimary())
}

func renderOpsCompact(status *opsSnapshot, loading bool, err error) string {
	if status == nil {
		if loading {
			return "OPS · reading cluster status…"
		}
		return "OPS · snapshot unavailable"
	}
	st := status.status
	api := opsAPIStatus(st)
	nodes := "Nodes unavailable"
	if st.NodesAvailable {
		ready := 0
		counts := nodeCounts(st.Nodes)
		for _, node := range st.Nodes {
			if node.Ready {
				ready++
			}
		}
		nodes = "Nodes " + strconv.Itoa(ready) + "/" + strconv.Itoa(counts.total) + " ready"
	}
	operators := "Operators unavailable"
	if st.OperatorsAvailable {
		operators = "Operators " + strconv.Itoa(st.DegradedOperators) + " degraded"
	}
	lastRun := "Last recorded run unavailable"
	if st.LastDeployRunID != "" && !st.LastDeployAt.IsZero() {
		lastRun = "Last recorded run " + st.LastDeployCluster + " · " + st.LastDeployRunID + " · " + formatOpsAge(status.updated, st.LastDeployAt)
	}
	prefix := "OPS"
	if err != nil {
		prefix += " · refresh failed"
	}
	return prefix + " · API " + api + "\n" + nodes + "\n" + operators + "\n" + lastRun + "\ns open full cluster status"
}

func opsAPIStatus(st *okd.ClusterStatus) string {
	if !st.APIAvailable {
		return "unavailable"
	}
	switch {
	case st.APIReachable && st.APILatencyAvailable:
		return "reachable · " + st.APILatency.Round(time.Millisecond).String()
	case st.APIReachable:
		return "reachable"
	case st.APILatencyAvailable:
		return "unreachable · probe " + st.APILatency.Round(time.Millisecond).String()
	default:
		return "unreachable"
	}
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

func joinOpsRows(rows []string) string {
	result := ""
	for i, row := range rows {
		if i > 0 {
			result += "\n"
		}
		result += row
	}
	return result
}

var errEmptyOpsSnapshot = errors.New("read operations snapshot: empty result")
