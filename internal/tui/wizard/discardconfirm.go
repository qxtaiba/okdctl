package wizard

import (
	"crypto/sha256"
	"encoding/json"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

const discardPrompt = "discard unsaved changes? ctrl+c or y quits · any other key stays"

// savedState is a digest of the config and secrets a flow opened with.
type savedState struct {
	digest [sha256.Size]byte
}

func newSavedState(cfg *config.Config, steps []WizardStep) savedState {
	return savedState{digest: sessionDigest(cfg, steps)}
}

func (s *savedState) matches(cfg *config.Config, steps []WizardStep) bool {
	return sessionDigest(cfg, steps) == s.digest
}

// sessionDigest hashes the config's JSON form plus, per SecretsProvider,
// whether a token is present and the TLS toggle; the token bytes never
// enter the hash.
func sessionDigest(cfg *config.Config, steps []WizardStep) [sha256.Size]byte {
	h := sha256.New()
	if cfg != nil {
		if data, err := json.Marshal(cfg); err == nil {
			h.Write(data)
		}
	}
	for _, step := range steps {
		p, ok := step.(SecretsProvider)
		if !ok {
			continue
		}
		secrets := p.Secrets()
		h.Write([]byte{boolByte(!secrets.ProxmoxToken.IsEmpty()), boolByte(secrets.ProxmoxInsecure)})
		secrets.ProxmoxToken.Zeroize()
	}
	return [sha256.Size]byte(h.Sum(nil))
}

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
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
	return step != nil && isConfigureStep(step.ID()) && !m.saved.matches(m.config, m.steps)
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
