package wizard

import (
	"crypto/sha256"
	"encoding/json"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

const discardPrompt = "discard unsaved changes? ctrl+c or y quits · any other key stays"

// configDigest hashes cfg's effective form plus the credentials that never
// serialize, so the model compares edits without keeping a copy of a secret.
func configDigest(cfg *config.Config) [sha256.Size]byte {
	h := sha256.New()
	if cfg == nil {
		return [sha256.Size]byte(h.Sum(nil))
	}
	if data, err := json.Marshal(config.Effective(cfg)); err == nil {
		h.Write(data)
	}
	if p := cfg.Provider.Proxmox; p != nil {
		for _, credential := range [][]byte{[]byte(p.Username), p.Password.Bytes(), p.APIToken.Bytes()} {
			h.Write([]byte{0})
			h.Write(credential)
		}
	}
	return [sha256.Size]byte(h.Sum(nil))
}

func isConfigureStep(id StepID) bool {
	switch id {
	case StepIDDistribution, StepIDBasics, StepIDProxmox, StepIDNodePlacement,
		StepIDNetworking, StepIDResources, StepIDAddons, StepIDFiles,
		StepIDAdvanced, StepIDReview:
		return true
	default:
		return false
	}
}

func (m *Model) asksBeforeDiscarding() bool {
	step := m.CurrentStep()
	return step != nil && isConfigureStep(step.ID()) && configDigest(m.config) != m.savedDigest
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
