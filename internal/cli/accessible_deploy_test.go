package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/wizarddraft"
)

type scriptedAccessiblePrompt struct {
	output        bytes.Buffer
	secretValue   string
	values        map[string]string
	stopAt        string
	stopWith      error
	linePrompts   []string
	secretPrompts []string
}

func (p *scriptedAccessiblePrompt) writer() io.Writer { return &p.output }

func (p *scriptedAccessiblePrompt) line(label, current string) (string, error) {
	p.linePrompts = append(p.linePrompts, label)
	if label == p.stopAt {
		return "", p.stopWith
	}
	if value, ok := p.values[label]; ok {
		return value, nil
	}
	if strings.HasPrefix(label, "OKD version") {
		return "4.18.0-okd-scos.1", nil
	}
	return current, nil
}

func (p *scriptedAccessiblePrompt) secret(label string) (string, error) {
	p.secretPrompts = append(p.secretPrompts, label)
	return p.secretValue, nil
}

func TestAccessibleDeployConfigureAppliesAndKeepsSecretsOutOfTranscript(t *testing.T) {
	cfg := config.DefaultConfig()
	filesDir := t.TempDir()
	pullSecret := filepath.Join(filesDir, "pull-secret.json")
	sshKey := filepath.Join(filesDir, "id_ed25519.pub")
	if err := os.WriteFile(pullSecret, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sshKey, []byte("ssh-ed25519 key"), 0o600); err != nil {
		t.Fatal(err)
	}
	prompt := &scriptedAccessiblePrompt{secretValue: "private-test-password", values: map[string]string{
		"pull secret (required)":    pullSecret,
		"ssh public key (required)": sshKey,
	}}
	draftPath := filepath.Join(t.TempDir(), "okdctl.yaml")
	saveDraft := func(cfg *config.Config, stepID wizard.StepID) error {
		return wizarddraft.New(draftPath).Save(cfg, wizarddraft.Cursor{StepID: stepID}, time.Now())
	}
	outcome, err := runAccessibleDeployConfigureWithDraft(context.Background(), prompt, cfg, false, saveDraft)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Result.Completed || outcome.Result.Action != wizard.ActionExit {
		t.Fatalf("outcome = %#v, want completed save action", outcome.Result)
	}
	if got := cfg.Distribution.Version; got != "4.18.0-okd-scos.1" {
		t.Fatalf("distribution version = %q", got)
	}
	if got := string(cfg.Provider.Proxmox.Password.Bytes()); got != prompt.secretValue {
		t.Fatalf("password = %q, want entered secret", got)
	}
	if strings.Contains(prompt.output.String(), prompt.secretValue) {
		t.Fatal("transcript contains password")
	}
	draft, err := os.ReadFile(draftPath + ".draft.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(draft), prompt.secretValue) {
		t.Fatal("draft contains secret input")
	}
	if len(prompt.secretPrompts) == 0 {
		t.Fatal("expected a no-echo secret prompt")
	}
	clearConfigCredentials(cfg)
}

func TestAccessibleDeployConfigureCancelsAndZeroizesSecret(t *testing.T) {
	cfg := config.DefaultConfig()
	prompt := &scriptedAccessiblePrompt{secretValue: "private-test-password"}
	prompt.stopAt = "machine cidr (required)"
	prompt.stopWith = io.EOF
	outcome, err := runAccessibleDeployConfigureWith(context.Background(), prompt, cfg, false)
	if err != nil {
		t.Fatalf("cancellation returned error: %v", err)
	}
	if !outcome.Result.Cancelled {
		t.Fatalf("outcome = %#v, want cancelled", outcome.Result)
	}
	if !cfg.Provider.Proxmox.Password.IsEmpty() {
		t.Fatal("cancelled secret remains in config")
	}
}
