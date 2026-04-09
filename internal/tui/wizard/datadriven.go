package wizard

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard/components"
)

type FieldType int

const (
	FieldTypeText FieldType = iota
	FieldTypePassword
	FieldTypeNumber
	FieldTypeBool // For yes/no fields
)

type ConfigSetter func(cfg *config.Config, value string) error
type ConfigGetter func(cfg *config.Config) string

type FieldDefinition struct {
	Key      string             // Unique key for accessing value
	Label    string             // Display label
	Default  string             // Default value (as string)
	Help     string             // Help text shown next to label
	Type     FieldType          // Field type
	Required bool               // Whether field is required
	Validate func(string) error // Optional validation function

	ConfigSet ConfigSetter
	ConfigGet ConfigGetter
}

type SectionDefinition struct {
	Title  string
	Note   string // Optional hint rendered below the title (e.g., prerequisites)
	Fields []FieldDefinition
}

type StepDefinition struct {
	ID           StepID
	Title        string
	DisplayTitle string
	Description  string
	Sections     []SectionDefinition

	Validate     func(values map[string]string) error
	Apply        func(step *DataDrivenStep, cfg *config.Config) error // Custom apply (after auto-binding)
	ShouldShow   func(*config.Config) bool
	ExtraContent func(values map[string]string, width int) string
}

type DataDrivenStep struct {
	*MultiFormStep
	definition StepDefinition
	fieldKeys  map[string]fieldLocation // maps Key -> section/field indices
}

type fieldLocation struct {
	section int
	field   int
}

func NewDataDrivenStep(def StepDefinition) *DataDrivenStep {
	formStep := NewMultiFormStep(def.ID, def.Title, def.DisplayTitle, def.Description)
	fieldKeys := make(map[string]fieldLocation)

	for sectionIdx, sectionDef := range def.Sections {
		fields := make([]components.FormField, 0, len(sectionDef.Fields))

		for fieldIdx, fieldDef := range sectionDef.Fields {
			fields = append(fields, buildInputField(fieldDef))
			fieldKeys[fieldDef.Key] = fieldLocation{
				section: sectionIdx,
				field:   fieldIdx,
			}
		}

		group := components.NewInputGroup(sectionDef.Title, fields...)
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
						return fmt.Errorf("field %s: %w", fieldDef.Key, err)
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

func buildInputField(def FieldDefinition) *components.InputField {
	var field *components.InputField
	if def.Type == FieldTypePassword {
		field = components.NewPasswordField(def.Label, def.Default)
	} else {
		field = components.NewInputField(def.Label, def.Default)
	}
	field.Required = def.Required
	field.Help = def.Help
	field.Validator = def.Validate
	return field
}

// getField resolves a field key to its FormField by walking the
// fieldKeys → Section → Group → Field chain, returning nil if any level
// is missing. Callers must handle the nil case.
func (s *DataDrivenStep) getField(key string) components.FormField {
	loc, ok := s.fieldKeys[key]
	if !ok {
		return nil
	}
	section := s.Section(loc.section)
	if section == nil || section.Group == nil {
		return nil
	}
	return section.Group.Field(loc.field)
}

func (s *DataDrivenStep) Values() map[string]string {
	values := make(map[string]string)
	for key := range s.fieldKeys {
		if field := s.getField(key); field != nil {
			values[key] = field.Value()
		}
	}
	return values
}

func (s *DataDrivenStep) Value(key string) string {
	if field := s.getField(key); field != nil {
		return field.Value()
	}
	return ""
}

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

func (s *DataDrivenStep) SetValues(values map[string]string) {
	for key, value := range values {
		s.SetValue(key, value)
	}
}

func (s *DataDrivenStep) LoadFromConfig(cfg *config.Config) {
	for _, sectionDef := range s.definition.Sections {
		for _, fieldDef := range sectionDef.Fields {
			if fieldDef.ConfigGet != nil {
				s.SetValue(fieldDef.Key, fieldDef.ConfigGet(cfg))
			}
		}
	}
}

func (s *DataDrivenStep) InputGroup(sectionIndex int) *components.InputGroup {
	section := s.Section(sectionIndex)
	if section != nil {
		return section.Group
	}
	return nil
}

func (s *DataDrivenStep) Definition() StepDefinition {
	return s.definition
}

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

func (s *DataDrivenStep) WithExtraContentFunc(fn func(step *DataDrivenStep, width int) string) *DataDrivenStep {
	s.WithExtraContent(func(width int) string {
		return fn(s, width)
	})
	return s
}

func SetString(setter func(cfg *config.Config, v string)) ConfigSetter {
	return func(cfg *config.Config, value string) error {
		setter(cfg, value)
		return nil
	}
}

func SetInt(setter func(cfg *config.Config, v int)) ConfigSetter {
	return func(cfg *config.Config, value string) error {
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer: %w", err)
		}
		setter(cfg, v)
		return nil
	}
}

func SetBool(setter func(cfg *config.Config, v bool)) ConfigSetter {
	return func(cfg *config.Config, value string) error {
		v := strings.ToLower(strings.TrimSpace(value))
		b := v == "yes" || v == "true" || v == "1" || v == "y"
		setter(cfg, b)
		return nil
	}
}

func GetString(getter func(cfg *config.Config) string) ConfigGetter {
	return getter
}

func GetInt(getter func(cfg *config.Config) int) ConfigGetter {
	return func(cfg *config.Config) string {
		return strconv.Itoa(getter(cfg))
	}
}

func GetBool(getter func(cfg *config.Config) bool) ConfigGetter {
	return func(cfg *config.Config) string {
		if getter(cfg) {
			return "yes"
		}
		return "no"
	}
}
