package wizard

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

const discardPrompt = "discard unsaved changes? ctrl+c or y quits · any other key stays"

// savedState is the config a flow opened with: a digest of everything that
// serializes, and the credentials themselves, which never do. A password
// must not go through a fast hash, so credentials are compared directly.
type savedState struct {
	digest   [sha256.Size]byte
	username string
	password []byte
	apiToken []byte
}

func newSavedState(cfg *config.Config) savedState {
	username, password, apiToken := credentialsOf(cfg)
	return savedState{
		digest:   configDigest(cfg),
		username: username,
		password: bytes.Clone(password),
		apiToken: bytes.Clone(apiToken),
	}
}

func (s *savedState) matches(cfg *config.Config) bool {
	username, password, apiToken := credentialsOf(cfg)
	return configDigest(cfg) == s.digest &&
		username == s.username &&
		subtle.ConstantTimeCompare(password, s.password) == 1 &&
		subtle.ConstantTimeCompare(apiToken, s.apiToken) == 1
}

func (s *savedState) release() {
	clear(s.password)
	clear(s.apiToken)
}

func credentialsOf(cfg *config.Config) (username string, password, apiToken []byte) {
	if cfg == nil || cfg.Provider.Proxmox == nil {
		return "", nil, nil
	}
	p := cfg.Provider.Proxmox
	return p.Username, p.Password.Bytes(), p.APIToken.Bytes()
}

func configDigest(cfg *config.Config) [sha256.Size]byte {
	if cfg == nil {
		return sha256.Sum256(nil)
	}
	data, err := json.Marshal(config.Effective(cfg))
	if err != nil {
		return sha256.Sum256(nil)
	}
	return sha256.Sum256(data)
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
