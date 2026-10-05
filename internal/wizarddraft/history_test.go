package wizarddraft

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func TestStoreSaveWithHistoryBoundsAndSanitizes(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "okdctl.yaml"))
	history := map[string][]string{
		"basics/cluster_name":  {"newest", "newer", "oldest", "password=sentinel", "-----BEGIN PRIVATE KEY-----"},
		"proxmox/password":     {"password-sentinel"},
		"proxmox/ssh_key":      {"ssh-ed25519 AAAA"},
		"welcome/cluster_name": {"utility"},
	}
	for i := 0; i < 12; i++ {
		history["networking/machine_cidr"] = append(history["networking/machine_cidr"], "value-"+string(rune('a'+i)))
	}
	if err := store.SaveWithHistory(config.DefaultConfig(), Cursor{StepID: wizard.StepIDBasics}, history, time.Now()); err != nil {
		t.Fatalf("SaveWithHistory: %v", err)
	}
	data, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"password-sentinel", "ssh-ed25519", "BEGIN PRIVATE KEY", "password=sentinel", "utility"} {
		if strings.Contains(string(data), forbidden) {
			t.Errorf("draft contains unsafe history value %q", forbidden)
		}
	}
	draft, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := draft.FieldHistory["basics/cluster_name"]; len(got) != 3 || got[0] != "newest" {
		t.Errorf("safe history = %v, want three safe values newest first", got)
	}
	if got := draft.FieldHistory["networking/machine_cidr"]; len(got) != 8 {
		t.Errorf("history length = %d, want max 8", len(got))
	}
}
