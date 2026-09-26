package steps

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

type reviewCheck struct {
	label   string
	status  string
	warning bool
}

func reviewPreflight(cfg *config.Config, capacity ...*WizardCapacitySnapshot) []reviewCheck {
	if cfg == nil {
		return nil
	}
	checks := []reviewCheck{
		{label: "pull secret", status: readableFileStatus(cfg.Files.PullSecret)},
		{label: "ssh public key", status: readableFileStatus(cfg.Files.SSHPublicKey)},
	}
	for i := range checks {
		checks[i].warning = checks[i].status == "unavailable"
	}

	networks := []string{
		cfg.Networking.MachineCIDR,
		cfg.Networking.PodCIDR,
		cfg.Networking.ServiceCIDR,
	}
	configuredNetworks := 0
	validNetworks := true
	for _, cidr := range networks {
		if cidr == "" {
			continue
		}
		configuredNetworks++
		if config.ValidateCIDR(cidr) != nil {
			validNetworks = false
		}
	}
	networkStatus := "not checked"
	networkWarning := false
	switch {
	case configuredNetworks == 0:
	case !validNetworks:
		networkStatus = "invalid CIDR"
		networkWarning = true
	case configuredNetworks != len(networks):
		networkStatus = "incomplete"
		networkWarning = true
	default:
		networkStatus = "no overlap"
		result := config.ValidateWithOptions(cfg, config.ValidationOptions{Scope: config.ScopeNetworking})
		for _, err := range result.Errors {
			if strings.Contains(err.Message, "overlaps with") {
				networkStatus = "overlap detected"
				networkWarning = true
				break
			}
		}
	}
	checks = append(checks, reviewCheck{label: "CIDR ranges", status: networkStatus, warning: networkWarning})
	if len(capacity) > 0 && capacity[0] != nil {
		checks = append(checks, reviewCapacityCheck(cfg, capacity[0]))
	}
	return checks
}

func reviewCapacityCheck(cfg *config.Config, snapshot *WizardCapacitySnapshot) reviewCheck {
	if cfg.Provider.Proxmox == nil {
		return reviewCheck{label: "selected capacity", status: "not applicable"}
	}
	nodes := snapshot.Nodes()
	if len(nodes) == 0 {
		return reviewCheck{label: "selected capacity", status: "not checked"}
	}
	byName := make(map[string]CapacityNode, len(nodes))
	for _, node := range nodes {
		byName[node.Name] = node
	}
	use := make(map[string]capacityDemand)
	add := func(node string, cpu, memoryMB int) {
		if node == "" {
			return
		}
		demand := use[node]
		demand.cpu += cpu
		demand.memoryMB += memoryMB
		use[node] = demand
	}
	p := cfg.Provider.Proxmox
	bootstrapCPU, bootstrapMemory := cfg.Topology.Bootstrap.CPU, cfg.Topology.Bootstrap.MemoryMB
	if bootstrapCPU == 0 && bootstrapMemory == 0 {
		bootstrapCPU = cfg.Topology.ControlPlane.CPU
		bootstrapMemory = cfg.Topology.ControlPlane.MemoryMB
	}
	add(p.Node, bootstrapCPU, bootstrapMemory)
	for _, node := range p.ControlPlaneNodes {
		add(node, cfg.Topology.ControlPlane.CPU, cfg.Topology.ControlPlane.MemoryMB)
	}
	for _, node := range p.WorkerNodes {
		add(node, cfg.Topology.Workers.CPU, cfg.Topology.Workers.MemoryMB)
	}
	if len(use) == 0 {
		return reviewCheck{label: "selected capacity", status: "not checked"}
	}
	rows := make([]string, 0, len(use))
	warning := false
	for _, node := range nodes {
		demand, assigned := use[node.Name]
		if !assigned {
			continue
		}
		cpu, memory := "?", "?"
		if node.CPUsKnown {
			cpu = fmt.Sprint(node.CPUs)
		}
		if node.MemoryKnown {
			memory = fmt.Sprintf("%d GB", node.MemoryGB)
		}
		over := (node.CPUsKnown && demand.cpu > node.CPUs) || (node.MemoryKnown && demand.memoryMB > node.MemoryGB*1024)
		state := "fits"
		switch {
		case node.Status != "online":
			state, warning = "offline", true
		case over:
			state, warning = "over capacity", true
		case !node.CPUsKnown || !node.MemoryKnown:
			state = "capacity unknown"
		}
		rows = append(rows, fmt.Sprintf("%s %s · %dc/%d GB → %sc/%s", node.Name, state, demand.cpu, demand.memoryMB/1024, cpu, memory))
	}
	for name := range use {
		if _, found := byName[name]; !found {
			rows = append(rows, name+" unavailable")
			warning = true
		}
	}
	slices.Sort(rows)
	return reviewCheck{label: "selected capacity", status: strings.Join(rows, "; "), warning: warning}
}

type capacityDemand struct {
	cpu      int
	memoryMB int
}

func readableFileStatus(path string) string {
	if path == "" {
		return "not configured"
	}
	path = system.ExpandPath(path)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "unavailable"
	}
	f, err := os.Open(path)
	if err != nil {
		return "unavailable"
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "unavailable"
	}
	return "readable"
}

func renderReviewPreflight(checks []reviewCheck, width int) string {
	if len(checks) == 0 {
		return ""
	}
	rows := make([]tui.FactRow, 0, len(checks))
	for _, check := range checks {
		rows = append(rows, tui.FactRow{Key: check.label, Value: check.status, Highlight: check.warning})
	}
	styles := tui.DefaultFactStyles()
	lines := tui.RenderFacts(rows, &tui.FactLayout{
		Leader:     tui.FactLeaderPad,
		KeyWidth:   20,
		TotalWidth: max(width-2, 1),
		Styles:     styles,
	})
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.ColorText()).Render("PREFLIGHT"))
	b.WriteString("\n")
	for _, line := range lines {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}
