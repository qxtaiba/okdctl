package steps

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/model"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

type reviewCheck struct {
	label         string
	status        string
	detail        string
	passed        bool
	warning       bool
	failed        bool // a required input is missing or invalid — blocks the install
	notApplicable bool
}

// reviewPreflight checks the two input files, runs cfg.Validate() and, with
// a discovery snapshot, the chosen hosts' capacity.
func reviewPreflight(cfg *config.Config, capacity ...*WizardCapacitySnapshot) []reviewCheck {
	if cfg == nil {
		return nil
	}
	checks := []reviewCheck{
		reviewFileCheck("pull secret", cfg.Files.PullSecret),
		reviewFileCheck("ssh public key", cfg.Files.SSHPublicKey),
		reviewConfigCheck(cfg),
	}
	if len(capacity) > 0 && capacity[0] != nil {
		checks = append(checks, reviewCapacityCheck(cfg, capacity[0]))
	}
	return checks
}

func reviewFileCheck(label, path string) reviewCheck {
	status := readableFileStatus(path)
	readable := status == "readable"
	return reviewCheck{label: label, status: status, detail: path, passed: readable, failed: !readable}
}

func reviewConfigCheck(cfg *config.Config) reviewCheck {
	result := cfg.Validate()
	if result.IsValid() {
		return reviewCheck{label: "configuration", status: "valid", passed: true}
	}
	return reviewCheck{
		label:  "configuration",
		status: fmt.Sprintf("%d error(s)", len(result.Errors)),
		detail: result.Error(),
		failed: true,
	}
}

// reviewCapacityCheck sums each chosen host's demand from the model's
// inventory against the discovered capacity.
func reviewCapacityCheck(cfg *config.Config, snapshot *WizardCapacitySnapshot) reviewCheck {
	if cfg.Provider.Type != config.ProviderProxmox {
		return reviewCheck{label: labelSelectedCapacity, status: "not applicable", notApplicable: true}
	}
	nodes := snapshot.nodes()
	if len(nodes) == 0 {
		return reviewCheck{label: labelSelectedCapacity, status: statusNotChecked}
	}
	inv, err := model.Build(cfg, "")
	if err != nil {
		return reviewCheck{label: labelSelectedCapacity, status: statusNotChecked, detail: err.Error()}
	}
	type demand struct{ cpu, memoryMiB int }
	use := make(map[string]*demand)
	for i := range inv.Nodes {
		n := &inv.Nodes[i]
		d := use[n.Host]
		if d == nil {
			d = &demand{}
			use[n.Host] = d
		}
		d.cpu += n.CPU
		d.memoryMiB += n.MemoryMiB
	}
	rows := make([]string, 0, len(use))
	warning, unknown := false, false
	for i := range nodes {
		node := &nodes[i]
		d, assigned := use[node.Name]
		if !assigned {
			continue
		}
		delete(use, node.Name)
		cpu, memory := "?", "?"
		if node.CPUsKnown {
			cpu = fmt.Sprint(node.CPUs)
		}
		if node.MemKnown {
			memory = fmt.Sprintf("%d GB", node.MemGB)
		}
		over := (node.CPUsKnown && d.cpu > node.CPUs) || (node.MemKnown && d.memoryMiB > node.MemGB*1024)
		state := "fits"
		switch {
		case node.Status != proxmoxStatusOnline:
			state, warning = "offline", true
		case over:
			state, warning = "over capacity", true
		case !node.CPUsKnown || !node.MemKnown:
			state, unknown = "capacity unknown", true
		}
		rows = append(rows, fmt.Sprintf("%s %s · %dc/%d GB → %sc/%s", tui.SanitizeTerminalEscapes(node.Name), state, d.cpu, d.memoryMiB/1024, cpu, memory))
	}
	for host := range use {
		rows = append(rows, tui.SanitizeTerminalEscapes(host)+" unavailable")
		warning = true
	}
	slices.Sort(rows)
	return reviewCheck{
		label: labelSelectedCapacity, status: strings.Join(rows, "; "),
		passed: !warning && !unknown, warning: warning,
	}
}

func readableFileStatus(path string) string {
	if path == "" {
		return "not configured"
	}
	path = system.ExpandPath(path)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return statusUnavailable
	}
	f, err := os.Open(path)
	if err != nil {
		return statusUnavailable
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return statusUnavailable
	}
	return "readable"
}

func renderReviewPreflight(checks []reviewCheck, width int) string {
	if len(checks) == 0 {
		return ""
	}
	passed, warnings, failed, unchecked := 0, 0, 0, 0
	for _, check := range checks {
		switch {
		case check.notApplicable:
		case check.failed:
			failed++
		case check.warning:
			warnings++
		case check.passed:
			passed++
		default:
			unchecked++
		}
	}
	var b strings.Builder
	header := fmt.Sprintf("PREFLIGHT · %d passed · %d warnings · %d failed · %d not checked", passed, warnings, failed, unchecked)
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.ColorText()).Render(tui.Truncate(header, width)))
	b.WriteString("\n")
	for _, check := range checks {
		icon, color := tui.IconPending, tui.ColorTextFaint()
		switch {
		case check.failed:
			icon, color = tui.IconError, tui.ColorError()
		case check.warning:
			icon, color = tui.IconWarning, tui.ColorWarning()
		case check.passed:
			icon, color = tui.IconSuccess, tui.ColorSuccess()
		}
		value := check.label + " · " + check.status
		if check.detail != "" {
			value += " · " + check.detail
		}
		for i, line := range tui.WrapLines(value, max(width-2, 1)) {
			if i == 0 {
				b.WriteString(lipgloss.NewStyle().Foreground(color).Render(icon))
				b.WriteString(" ")
			} else {
				b.WriteString("  ")
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}
