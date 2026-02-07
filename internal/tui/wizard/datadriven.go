// Package wizard provides the multi-step configuration wizard.
package wizard

import (
	"strconv"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard/components"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

// FieldType represents the type of input field.
type FieldType int

const (
	FieldTypeText FieldType = iota
	FieldTypePassword
	FieldTypeNumber
	FieldTypeBool // For yes/no fields
)

// ConfigSetter is a function that sets a config value from a string.
type ConfigSetter func(cfg *config.Config, value string) error

// ConfigGetter is a function that gets a config value as a string.
type ConfigGetter func(cfg *config.Config) string

// FieldDefinition describes a single form field declaratively.
type FieldDefinition struct {
	Key      string              // Unique key for accessing value
	Label    string              // Display label
	Default  string              // Default value (as string)
	Help     string              // Help text shown next to label
	Type     FieldType           // Field type
	Required bool                // Whether field is required
	Validate func(string) error // Optional validation function

	// Config binding (optional - if set, auto-apply/load works)
	ConfigSet ConfigSetter
	ConfigGet ConfigGetter
}

// SectionDefinition describes a group of related fields.
type SectionDefinition struct {
	Title  string
	Note   string // Optional hint rendered below the title (e.g., prerequisites)
	Fields []FieldDefinition
}

// StepDefinition describes a wizard step declaratively.
type StepDefinition struct {
	ID           StepID
	Title        string
	DisplayTitle string
	Description  string
	Sections     []SectionDefinition

	// Optional callbacks for step-specific behavior
	Validate     func(values map[string]string) error
	Apply        func(step *DataDrivenStep, cfg *config.Config) error // Custom apply (after auto-binding)
	ShouldShow   func(*config.Config) bool
	ExtraContent func(values map[string]string, width int) string
}

// DataDrivenStep is a step created from a StepDefinition.
// It wraps MultiFormStep and provides value access by key.
type DataDrivenStep struct {
	*MultiFormStep
	definition StepDefinition
	fieldKeys  map[string]fieldLocation // maps Key -> section/field indices
}

// fieldLocation stores the location of a field within sections.
type fieldLocation struct {
	section int
	field   int
}

// NewDataDrivenStep creates a wizard step from a declarative definition.
func NewDataDrivenStep(def StepDefinition) *DataDrivenStep {
	formStep := NewMultiFormStep(def.ID, def.Title, def.DisplayTitle, def.Description)
	fieldKeys := make(map[string]fieldLocation)

	for sectionIdx, sectionDef := range def.Sections {
		sectionBuilder := NewSectionBuilder(sectionDef.Title)

		for fieldIdx, fieldDef := range sectionDef.Fields {
			fb := createFieldBuilder(fieldDef)
			sectionBuilder = sectionBuilder.Field(fb)
			fieldKeys[fieldDef.Key] = fieldLocation{
				section: sectionIdx,
				field:   fieldIdx,
			}
		}

		group := sectionBuilder.Build()
		if sectionDef.Note != "" {
			formStep = formStep.AddSectionWithNote(sectionDef.Title, sectionDef.Note, group)
		} else {
			formStep = formStep.AddSection(sectionDef.Title, group)
		}
	}

	step := &DataDrivenStep{
		MultiFormStep: formStep,
		definition:    def,
		fieldKeys:     fieldKeys,
	}

	if def.Validate != nil {
		formStep.WithValidation(func() error {
			return def.Validate(step.Values())
		})
	}

	if def.ShouldShow != nil {
		formStep.WithShouldShow(def.ShouldShow)
	}

	if def.ExtraContent != nil {
		formStep.WithExtraContent(func(width int) string {
			return def.ExtraContent(step.Values(), width)
		})
	}

	formStep.WithApply(func(cfg *config.Config) error {
		for _, sectionDef := range def.Sections {
			for _, fieldDef := range sectionDef.Fields {
				if fieldDef.ConfigSet != nil {
					value := step.Value(fieldDef.Key)
					if err := fieldDef.ConfigSet(cfg, value); err != nil {
						return utils.WrapErrorf(err, "field %s", fieldDef.Key)
					}
				}
			}
		}
		if def.Apply != nil {
			return def.Apply(step, cfg)
		}
		return nil
	})

	return step
}

func createFieldBuilder(def FieldDefinition) *FieldBuilder {
	var fb *FieldBuilder

	switch def.Type {
	case FieldTypePassword:
		fb = PasswordField(def.Label, def.Default)
	default:
		fb = TextField(def.Label, def.Default)
	}

	if def.Required {
		fb = fb.Required()
	}
	if def.Help != "" {
		fb = fb.Help(def.Help)
	}
	if def.Validate != nil {
		fb = fb.Validate(def.Validate)
	}

	return fb
}

// Values returns all field values as a map keyed by field Key.
func (s *DataDrivenStep) Values() map[string]string {
	values := make(map[string]string)
	for key, loc := range s.fieldKeys {
		section := s.Section(loc.section)
		if section != nil && section.Group != nil {
			field := section.Group.Field(loc.field)
			if field != nil {
				values[key] = field.Value()
			}
		}
	}
	return values
}

// Value returns a single field value by key.
func (s *DataDrivenStep) Value(key string) string {
	loc, ok := s.fieldKeys[key]
	if !ok {
		return ""
	}
	section := s.Section(loc.section)
	if section != nil && section.Group != nil {
		field := section.Group.Field(loc.field)
		if field != nil {
			return field.Value()
		}
	}
	return ""
}

// SetValue sets a field value by key.
func (s *DataDrivenStep) SetValue(key, value string) {
	loc, ok := s.fieldKeys[key]
	if !ok {
		return
	}
	section := s.Section(loc.section)
	if section != nil && section.Group != nil {
		field := section.Group.Field(loc.field)
		if field != nil {
			field.SetValue(value)
		}
	}
}

// SetValues sets multiple field values from a map.
func (s *DataDrivenStep) SetValues(values map[string]string) {
	for key, value := range values {
		s.SetValue(key, value)
	}
}

// LoadFromConfig populates field values from config using ConfigGet bindings.
func (s *DataDrivenStep) LoadFromConfig(cfg *config.Config) {
	for _, sectionDef := range s.definition.Sections {
		for _, fieldDef := range sectionDef.Fields {
			if fieldDef.ConfigGet != nil {
				value := fieldDef.ConfigGet(cfg)
				if value != "" {
					s.SetValue(fieldDef.Key, value)
				}
			}
		}
	}
}

// InputGroup returns the InputGroup for a section by index.
// Useful for step-specific state access.
func (s *DataDrivenStep) InputGroup(sectionIndex int) *components.InputGroup {
	section := s.Section(sectionIndex)
	if section != nil {
		return section.Group
	}
	return nil
}

// Definition returns the step's definition.
func (s *DataDrivenStep) Definition() StepDefinition {
	return s.definition
}

// ValueInt returns a field value as int, with fallback.
func (s *DataDrivenStep) ValueInt(key string, fallback int) int {
	v := s.Value(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}

// ValueBool returns a field value as bool (yes/true/1 = true).
func (s *DataDrivenStep) ValueBool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(s.Value(key)))
	return v == "yes" || v == "true" || v == "1" || v == "y"
}

// SetValueInt sets a field value from an int.
func (s *DataDrivenStep) SetValueInt(key string, value int) {
	s.SetValue(key, strconv.Itoa(value))
}

// SetValueBool sets a field value from a bool (as "yes"/"no").
func (s *DataDrivenStep) SetValueBool(key string, value bool) {
	if value {
		s.SetValue(key, "yes")
	} else {
		s.SetValue(key, "no")
	}
}

// WithApplyFunc sets a custom Apply callback that runs after auto-binding.
func (s *DataDrivenStep) WithApplyFunc(fn func(step *DataDrivenStep, cfg *config.Config) error) *DataDrivenStep {
	s.definition.Apply = fn
	return s
}

// WithExtraContentFunc sets extra content to render after sections.
func (s *DataDrivenStep) WithExtraContentFunc(fn func(step *DataDrivenStep, width int) string) *DataDrivenStep {
	s.WithExtraContent(func(width int) string {
		return fn(s, width)
	})
	return s
}

// SetString creates a ConfigSetter for a string field.
func SetString(setter func(cfg *config.Config, v string)) ConfigSetter {
	return func(cfg *config.Config, value string) error {
		setter(cfg, value)
		return nil
	}
}

// SetInt creates a ConfigSetter for an integer field.
func SetInt(setter func(cfg *config.Config, v int)) ConfigSetter {
	return func(cfg *config.Config, value string) error {
		v, err := strconv.Atoi(value)
		if err != nil {
			return utils.WrapError("invalid integer", err)
		}
		setter(cfg, v)
		return nil
	}
}

// SetBool creates a ConfigSetter for a boolean field (yes/no).
func SetBool(setter func(cfg *config.Config, v bool)) ConfigSetter {
	return func(cfg *config.Config, value string) error {
		v := strings.ToLower(strings.TrimSpace(value))
		b := v == "yes" || v == "true" || v == "1" || v == "y"
		setter(cfg, b)
		return nil
	}
}

// GetString creates a ConfigGetter for a string field.
func GetString(getter func(cfg *config.Config) string) ConfigGetter {
	return getter
}

// GetInt creates a ConfigGetter for an integer field.
func GetInt(getter func(cfg *config.Config) int) ConfigGetter {
	return func(cfg *config.Config) string {
		v := getter(cfg)
		if v == 0 {
			return ""
		}
		return strconv.Itoa(v)
	}
}

// GetBool creates a ConfigGetter for a boolean field.
func GetBool(getter func(cfg *config.Config) bool) ConfigGetter {
	return func(cfg *config.Config) string {
		if getter(cfg) {
			return "yes"
		}
		return "no"
	}
}
