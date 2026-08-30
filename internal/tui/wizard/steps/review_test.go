package steps

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func reviewTestConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Provider.Proxmox = &config.ProxmoxConfig{
		Host:              "pve.local",
		ControlPlaneNodes: []string{"pve1", "pve2", "pve3"},
		WorkerNodes:       []string{"pve1", "pve2"},
	}
	cfg.Addons = map[string]config.AddonConfig{"flux": {Enabled: true}}
	cfg.Deployment.AutoApprove = true
	return cfg
}

func TestReviewStep_HiddenStepGetsNoIndex(t *testing.T) {
	s := NewReviewStep()
	s.SetConfig(reviewTestConfig())

	// files omitted, simulating a step hidden by ShouldShow — header must render without a digit.
	s.SetJumpTargets([]wizard.JumpTarget{
		{StepID: wizard.StepIDBasics, Digit: 1},
		{StepID: wizard.StepIDProxmox, Digit: 2},
	})

	out := s.View(100, 100)
	if !strings.Contains(out, "[2] proxmox") {
		t.Error("View() dropped the jump index for a mapped step")
	}
	if strings.Contains(out, "] files & ignition") {
		t.Error("View() shows a jump index for files & ignition, want none (hidden step)")
	}
	if !strings.Contains(out, "files & ignition") {
		t.Error("View() dropped the files & ignition header entirely, want header without an index")
	}
}

func TestReviewStep_DigitKeyEmitsJumpToStepMsg(t *testing.T) {
	s := NewReviewStep()
	s.SetJumpTargets([]wizard.JumpTarget{
		{StepID: wizard.StepIDProxmox, Digit: 2},
	})

	_, cmd := s.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	if cmd == nil {
		t.Fatal("Update(digit 2) returned a nil cmd, want a JumpToStepMsg command")
	}
	msg := cmd()
	jump, ok := msg.(wizard.JumpToStepMsg)
	if !ok {
		t.Fatalf("Update(digit 2) cmd produced %T, want wizard.JumpToStepMsg", msg)
	}
	if jump.StepID != wizard.StepIDProxmox {
		t.Errorf("JumpToStepMsg.StepID = %v, want StepIDProxmox", jump.StepID)
	}
}

func TestReviewStep_UnmappedDigitDoesNotJump(t *testing.T) {
	s := NewReviewStep()
	s.SetJumpTargets([]wizard.JumpTarget{
		{StepID: wizard.StepIDProxmox, Digit: 2},
	})

	// Digit 9 has no target; the keypress must fall through, not be swallowed.
	_, cmd := s.Update(tea.KeyPressMsg{Code: '9', Text: "9"})
	if cmd == nil {
		return
	}
	if _, ok := cmd().(wizard.JumpToStepMsg); ok {
		t.Fatal("Update(unmapped digit) produced a JumpToStepMsg, want none")
	}
}

func TestReviewStep_ShortHelp_AdvertisesJumpOnlyWhenTargetsExist(t *testing.T) {
	s := NewReviewStep()

	for _, b := range s.ShortHelp() {
		if b.Help == wizard.HelpJump {
			t.Fatal("ShortHelp() advertises jump before any targets are set")
		}
	}

	s.SetJumpTargets([]wizard.JumpTarget{{StepID: wizard.StepIDProxmox, Digit: 1}})

	found := false
	for _, b := range s.ShortHelp() {
		if b.Help == wizard.HelpJump {
			found = true
		}
	}
	if !found {
		t.Error("ShortHelp() does not advertise jump once targets are set")
	}
}
