package steps

import (
	"fmt"
	"os"
	"os/exec"
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
	detail  string
	passed  bool
	warning bool
}

func reviewPreflight(cfg *config.Config, capacity ...*WizardCapacitySnapshot) []reviewCheck {
	if cfg == nil {
		return nil
	}
	checks := []reviewCheck{
		reviewFileCheck("pull secret", cfg.Files.PullSecret),
		reviewFileCheck("ssh public key", cfg.Files.SSHPublicKey),
	}
	if cfg.Addons["flux"].Enabled {
		check := reviewFileCheck("flux deploy key", system.ExpandPath("~/.ssh/flux-deploy-key"))
		check.detail = "~/.ssh/flux-deploy-key"
		checks = append(checks, check)
	}
	if cfg.Addons["secretstore"].Enabled {
		_, err := exec.LookPath("sops")
		check := reviewCheck{label: "sops", status: "available", passed: err == nil}
		if err != nil {
			check.status, check.warning = "not found", true
		}
		checks = append(checks, check)
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
	networkStatus := statusNotChecked
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
	checks = append(checks, reviewCheck{
		label: "CIDR ranges", status: networkStatus,
		detail: strings.Join(configuredCIDRs(networks), " · "),
		passed: networkStatus == "no overlap", warning: networkWarning,
	})
	if len(capacity) > 0 && capacity[0] != nil {
		checks = append(checks, reviewCapacityCheck(cfg, capacity[0]))
	}
	return checks
}

func configuredCIDRs(networks []string) []string {
	configured := make([]string, 0, len(networks))
	for _, network := range networks {
		if network != "" {
			configured = append(configured, network)
		}
	}
	return configured
}

func reviewFileCheck(label, path string) reviewCheck {
	status := readableFileStatus(path)
	return reviewCheck{
		label: label, status: status, detail: path, passed: status == "readable",
		warning: status == statusUnavailable,
	}
}

func reviewCapacityCheck(cfg *config.Config, snapshot *WizardCapacitySnapshot) reviewCheck {
	if cfg.Provider.Proxmox == nil {
		return reviewCheck{label: labelSelectedCapacity, status: "not applicable"}
	}
	nodes := snapshot.Nodes()
	if len(nodes) == 0 {
		return reviewCheck{label: labelSelectedCapacity, status: statusNotChecked}
	}
	byName := make(map[string]*CapacityNode, len(nodes))
	for i := range nodes {
		node := &nodes[i]
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
		return reviewCheck{label: labelSelectedCapacity, status: statusNotChecked}
	}
	rows := make([]string, 0, len(use))
	warning := false
	unknown := false
	for nodeIndex := range nodes {
		node := &nodes[nodeIndex]
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
			state, unknown = "capacity unknown", true
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
	return reviewCheck{
		label: labelSelectedCapacity, status: strings.Join(rows, "; "),
		passed: !warning && !unknown, warning: warning,
	}
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
	passed, warnings, unchecked := 0, 0, 0
	for _, check := range checks {
		switch {
		case check.warning:
			warnings++
		case check.passed:
			passed++
		default:
			unchecked++
		}
	}
	var b strings.Builder
	header := fmt.Sprintf("PREFLIGHT · %d passed · %d warnings · %d not checked", passed, warnings, unchecked)
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.ColorText()).Render(tui.Truncate(header, width)))
	b.WriteString("\n")
	for _, check := range checks {
		icon, color := tui.IconPending, tui.ColorTextFaint()
		switch {
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
