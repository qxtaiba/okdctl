package wizard

import (
	"strconv"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard/components"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
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
	Key      string              // Unique key for accessing value
	Label    string              // Display label
	Default  string              // Default value (as string)
	Help     string              // Help text shown next to label
	Type     FieldType           // Field type
	Required bool                // Whether field is required
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
				value := fieldDef.ConfigGet(cfg)
				if value != "" {
					s.SetValue(fieldDef.Key, value)
				}
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

func (s *DataDrivenStep) ValueBool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(s.Value(key)))
	return v == "yes" || v == "true" || v == "1" || v == "y"
}

func (s *DataDrivenStep) SetValueInt(key string, value int) {
	s.SetValue(key, strconv.Itoa(value))
}

func (s *DataDrivenStep) SetValueBool(key string, value bool) {
	if value {
		s.SetValue(key, "yes")
	} else {
		s.SetValue(key, "no")
	}
}

func (s *DataDrivenStep) WithApplyFunc(fn func(step *DataDrivenStep, cfg *config.Config) error) *DataDrivenStep {
	s.definition.Apply = fn
	return s
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
			return utils.WrapError("invalid integer", err)
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
		v := getter(cfg)
		if v == 0 {
			return ""
		}
		return strconv.Itoa(v)
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
