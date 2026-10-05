// Package wizarddraft stores resumable configuration wizard progress.
package wizarddraft

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

const schemaVersion = 1

// Cursor identifies the configure step and safe field that should regain focus.
type Cursor struct {
	StepID   wizard.StepID `json:"step_id"`
	FieldKey string        `json:"field_key,omitempty"`
}

// Draft contains safe configuration values and the wizard resume cursor.
type Draft struct {
	Config    *config.Config `json:"config"`
	Cursor    Cursor         `json:"cursor"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// Store writes versioned drafts beside their associated config file.
type Store struct {
	path string
}

type document struct {
	Version       int            `json:"version"`
	ConfigVersion string         `json:"config_version"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Cursor        Cursor         `json:"cursor"`
	Config        *config.Config `json:"config"`
}

// New returns the draft store associated with configPath.
func New(configPath string) *Store {
	return &Store{path: configPath + ".draft.json"}
}

// Path returns the sidecar path used by s.
func (s *Store) Path() string { return s.path }

// Save atomically stores cfg without credential fields or secret-keyed settings.
func (s *Store) Save(cfg *config.Config, cursor Cursor, now time.Time) error {
	if cfg == nil {
		return errors.New("save draft: nil config")
	}
	if !supportedStep(cursor.StepID) {
		return fmt.Errorf("save draft: unsupported step %q", cursor.StepID)
	}
	cursor.FieldKey = safeFieldKey(cursor.FieldKey)

	safeCfg, err := safeConfig(cfg)
	if err != nil {
		return fmt.Errorf("save draft: prepare config: %w", err)
	}
	data, err := json.Marshal(document{
		Version:       schemaVersion,
		ConfigVersion: config.SchemaVersionCurrent,
		UpdatedAt:     now.UTC(),
		Cursor:        cursor,
		Config:        safeCfg,
	})
	if err != nil {
		return fmt.Errorf("save draft: marshal: %w", err)
	}
	if err := system.AtomicWrite(s.path, data, 0o600); err != nil {
		return fmt.Errorf("save draft: %w", err)
	}
	return nil
}

// Load reads a valid draft; malformed, unknown, and incompatible files return an error.
func (s *Store) Load() (*Draft, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read draft: %w", err)
	}

	var doc document
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse draft: %w", err)
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("parse draft: multiple JSON values")
		}
		return nil, fmt.Errorf("parse draft: %w", err)
	}
	if doc.Version != schemaVersion {
		return nil, fmt.Errorf("load draft: unsupported version %d", doc.Version)
	}
	if doc.ConfigVersion != config.SchemaVersionCurrent {
		return nil, fmt.Errorf("load draft: unsupported config version %q", doc.ConfigVersion)
	}
	if doc.Config == nil || !supportedStep(doc.Cursor.StepID) || doc.UpdatedAt.IsZero() {
		return nil, errors.New("load draft: missing config, cursor, or timestamp")
	}
	if doc.Config.SchemaVersion != config.SchemaVersionCurrent {
		return nil, fmt.Errorf("load draft: config schema version %q is incompatible", doc.Config.SchemaVersion)
	}
	safeCfg, err := safeConfig(doc.Config)
	if err != nil {
		return nil, fmt.Errorf("load draft: sanitize config: %w", err)
	}
	doc.Cursor.FieldKey = safeFieldKey(doc.Cursor.FieldKey)
	return &Draft{Config: safeCfg, Cursor: doc.Cursor, UpdatedAt: doc.UpdatedAt}, nil
}

// Clear removes the draft and succeeds when it is already absent.
func (s *Store) Clear() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear draft: %w", err)
	}
	return nil
}

func safeConfig(cfg *config.Config) (*config.Config, error) {
	clone := *cfg
	if clone.Provider.Proxmox != nil {
		proxmox := *clone.Provider.Proxmox
		proxmox.Username = ""
		proxmox.Password = config.SecretBytes{}
		proxmox.APIToken = config.SecretBytes{}
		proxmox.TokenID = ""
		clone.Provider.Proxmox = &proxmox
	}
	if len(clone.Addons) > 0 {
		addons := make(map[string]config.AddonConfig, len(clone.Addons))
		for name, addon := range clone.Addons {
			if len(addon.Settings) > 0 {
				settings := make(map[string]string, len(addon.Settings))
				for key, value := range addon.Settings {
					if !sensitiveKey(key) {
						settings[key] = value
					}
				}
				addon.Settings = settings
			}
			addons[name] = addon
		}
		clone.Addons = addons
	}

	data, err := json.Marshal(&clone)
	if err != nil {
		return nil, err
	}
	var safe config.Config
	if err := json.Unmarshal(data, &safe); err != nil {
		return nil, err
	}
	return &safe, nil
}

func supportedStep(id wizard.StepID) bool {
	switch id {
	case wizard.StepIDDistribution, wizard.StepIDBasics, wizard.StepIDProxmox,
		wizard.StepIDNodePlacement, wizard.StepIDNetworking, wizard.StepIDResources,
		wizard.StepIDAddons, wizard.StepIDFiles, wizard.StepIDAdvanced, wizard.StepIDReview:
		return true
	default:
		return false
	}
}

func safeFieldKey(key string) string {
	if sensitiveKey(key) {
		return ""
	}
	return key
}

func sensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	return logutil.KeyIsSecret(key) || strings.Contains(lower, "username") ||
		strings.Contains(lower, "private_key") || strings.Contains(lower, "privatekey")
}
