package wizard

import (
	"crypto/sha256"
	"encoding/json"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

const discardPrompt = "discard unsaved changes? ctrl+c or y quits · any other key stays"

// savedState is a digest of the config a flow opened with.
type savedState struct {
	digest [sha256.Size]byte
}

func newSavedState(cfg *config.Config) savedState {
	return savedState{digest: configDigest(cfg)}
}

func (s *savedState) matches(cfg *config.Config) bool {
	return configDigest(cfg) == s.digest
}

func configDigest(cfg *config.Config) [sha256.Size]byte {
	if cfg == nil {
		return sha256.Sum256(nil)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return sha256.Sum256(nil)
	}
	return sha256.Sum256(data)
}

func isConfigureStep(id StepID) bool {
	switch id {
	case StepIDDistribution, StepIDBasics, StepIDProxmox, StepIDNodePlacement,
		StepIDNetworking, StepIDResources, StepIDFiles,
		StepIDAdvanced, StepIDReview:
		return true
	default:
		return false
	}
}

func (m *Model) asksBeforeDiscarding() bool {
	step := m.CurrentStep()
	return step != nil && isConfigureStep(step.ID()) && !m.saved.matches(m.config)
}

func (m *Model) handleDiscardKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	m.discardPending = false
	if strings.EqualFold(msg.Text, "y") {
		return m.cancel()
	}
	return m, nil, true
}

func (m *Model) cancel() (tea.Model, tea.Cmd, bool) {
	m.quitting = true
	m.result = Result{Outcome: OutcomeCancelled}
	return m, tea.Quit, true
}
