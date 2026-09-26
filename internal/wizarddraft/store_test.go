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

func TestStoreSaveLoadSanitizesCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "okdctl.yaml")
	store := New(path)
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "draft-cluster"
	cfg.Provider.Proxmox = &config.ProxmoxConfig{
		Host:     "pve.example.test",
		Username: "username-sentinel",
		TokenID:  "token-id-sentinel",
	}
	cfg.Provider.Proxmox.Password.Set("password-sentinel")
	cfg.Provider.Proxmox.APIToken.Set("api-token-sentinel")
	cfg.Addons = map[string]config.AddonConfig{
		"sample": {Enabled: true, Settings: map[string]string{
			"api_token":   "addon-secret-sentinel",
			"username":    "addon-user-sentinel",
			"private_key": "addon-private-key-sentinel",
			"mode":        "safe",
		}},
	}

	updatedAt := time.Date(2026, time.September, 26, 10, 30, 0, 0, time.UTC)
	if err := store.Save(cfg, Cursor{StepID: wizard.StepIDNetworking, FieldKey: "machine_cidr"}, updatedAt); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if string(cfg.Provider.Proxmox.Password.Bytes()) != "password-sentinel" ||
		string(cfg.Provider.Proxmox.APIToken.Bytes()) != "api-token-sentinel" {
		t.Fatal("saving a draft mutated the live credential buffers")
	}

	data, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, forbidden := range []string{
		"username-sentinel", "token-id-sentinel", "password-sentinel",
		"api-token-sentinel", "addon-secret-sentinel", "addon-user-sentinel",
		"addon-private-key-sentinel",
	} {
		if strings.Contains(string(data), forbidden) {
			t.Errorf("draft contains credential %q: %s", forbidden, data)
		}
	}
	if !strings.Contains(string(data), `"mode":"safe"`) {
		t.Errorf("safe configuration value was not persisted: %s", data)
	}
	if info, err := os.Stat(store.Path()); err != nil {
		t.Fatalf("Stat: %v", err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("draft mode = %#o, want 0600", got)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned no draft")
	}
	if got.Cursor.StepID != wizard.StepIDNetworking || got.Cursor.FieldKey != "machine_cidr" {
		t.Errorf("cursor = %+v, want networking/machine_cidr", got.Cursor)
	}
	if !got.UpdatedAt.Equal(updatedAt) {
		t.Errorf("UpdatedAt = %s, want %s", got.UpdatedAt, updatedAt)
	}
	if got.Config.Cluster.Name != "draft-cluster" || got.Config.Provider.Proxmox.Host != "pve.example.test" {
		t.Errorf("safe configuration did not round-trip: %+v", got.Config)
	}
	if got.Config.Provider.Proxmox.Username != "" || got.Config.Provider.Proxmox.TokenID != "" ||
		!got.Config.Provider.Proxmox.Password.IsEmpty() || !got.Config.Provider.Proxmox.APIToken.IsEmpty() {
		t.Errorf("credentials were loaded from draft: %+v", got.Config.Provider.Proxmox)
	}
	if got.Config.Addons["sample"].Settings["api_token"] != "" ||
		got.Config.Addons["sample"].Settings["username"] != "" ||
		got.Config.Addons["sample"].Settings["private_key"] != "" ||
		got.Config.Addons["sample"].Settings["mode"] != "safe" {
		t.Errorf("addon settings were not scrubbed safely: %+v", got.Config.Addons["sample"].Settings)
	}
}

func TestStoreLoadMissingDraft(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "okdctl.yaml"))
	got, err := store.Load()
	if err != nil || got != nil {
		t.Fatalf("Load() = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestStoreLoadRejectsCorruptAndIncompatibleDrafts(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{name: "corrupt", data: `{`},
		{name: "incompatible version", data: `{"version":99}`},
		{name: "unknown field", data: `{"version":1,"unknown":"value"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := New(filepath.Join(t.TempDir(), "okdctl.yaml"))
			if err := os.WriteFile(store.Path(), []byte(tc.data), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := store.Load()
			if err == nil || got != nil {
				t.Fatalf("Load() = (%v, %v), want nil draft and an error", got, err)
			}
			if _, statErr := os.Stat(store.Path()); statErr != nil {
				t.Errorf("invalid draft should be preserved for diagnosis: %v", statErr)
			}
		})
	}
}

func TestStoreRejectsUnsafeCursor(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "okdctl.yaml"))
	cfg := config.DefaultConfig()
	if err := store.Save(cfg, Cursor{StepID: wizard.StepIDBasics, FieldKey: "api_token"}, time.Now()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	draft, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if draft.Cursor.FieldKey != "" {
		t.Errorf("secret field cursor = %q, want empty", draft.Cursor.FieldKey)
	}

	if err := store.Save(cfg, Cursor{StepID: "unknown-step"}, time.Now()); err == nil {
		t.Fatal("Save accepted an unknown step")
	}
}

func TestStoreClearIsIdempotent(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "okdctl.yaml"))
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear missing draft: %v", err)
	}
	if err := store.Save(config.DefaultConfig(), Cursor{StepID: wizard.StepIDBasics}, time.Now()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Errorf("draft still exists after Clear: %v", err)
	}
}
