package steps

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func TestReviewStep_ValidateAndApplyAreNoOps(t *testing.T) {
	s := NewReviewStep()
	if err := s.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}

	cfg := &config.Config{}
	cfg.Cluster.Name = "unchanged"
	if err := s.Apply(cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if cfg.Cluster.Name != "unchanged" {
		t.Errorf("Apply mutated cfg: Cluster.Name = %q", cfg.Cluster.Name)
	}
}

func TestReviewStep_GetSelectedAction_DefaultsToDeploy(t *testing.T) {
	s := NewReviewStep()
	if got := s.GetSelectedAction(); got != wizard.ActionDeploy {
		t.Errorf("GetSelectedAction() default = %v, want ActionDeploy", got)
	}
}

func TestReviewStep_JumpOrder(t *testing.T) {
	s := NewReviewStep()
	want := []wizard.StepID{
		wizard.StepIDBasics,
		wizard.StepIDProxmox,
		wizard.StepIDNodePlacement,
		wizard.StepIDNetworking,
		wizard.StepIDResources,
		wizard.StepIDFiles,
		wizard.StepIDAddons,
		wizard.StepIDAdvanced,
	}
	got := s.JumpOrder()
	if len(got) != len(want) {
		t.Fatalf("JumpOrder() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("JumpOrder()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

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

func TestReviewStep_SectionHeadersShowJumpIndex(t *testing.T) {
	s := NewReviewStep()
	s.SetConfig(reviewTestConfig())
	s.SetJumpTargets([]wizard.JumpTarget{
		{StepID: wizard.StepIDBasics, Digit: 1},
		{StepID: wizard.StepIDProxmox, Digit: 2},
		{StepID: wizard.StepIDNodePlacement, Digit: 3},
		{StepID: wizard.StepIDNetworking, Digit: 4},
		{StepID: wizard.StepIDResources, Digit: 5},
		{StepID: wizard.StepIDFiles, Digit: 6},
		{StepID: wizard.StepIDAddons, Digit: 7},
		{StepID: wizard.StepIDAdvanced, Digit: 8},
	})

	out := s.View(100, 100)

	for _, want := range []string{
		"[1] cluster identity",
		"[2] proxmox",
		"[3] node placement",
		"[4] networking",
		"[5] compute",
		"[6] files & ignition",
		"[7] addons",
		"[8] advanced",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("View() missing %q", want)
		}
	}
}

func TestReviewStep_HiddenStepGetsNoIndex(t *testing.T) {
	s := NewReviewStep()
	s.SetConfig(reviewTestConfig())

	// files omitted, simulating a step hidden by ShouldShow (e.g.
	// distribution != OKD): its header must render without a digit, and no
	// other section may claim its digit.
	s.SetJumpTargets([]wizard.JumpTarget{
		{StepID: wizard.StepIDBasics, Digit: 1},
		{StepID: wizard.StepIDProxmox, Digit: 2},
	})

	out := s.View(100, 100)
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

	// Digit 9 has no target; the keypress must fall through to the action
	// selector instead of being swallowed as a jump.
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
