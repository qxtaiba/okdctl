package wizard

import (
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard/components"
)

// FieldBuilder provides a fluent interface for creating InputFields.
// It reduces boilerplate when defining wizard form fields by chaining
// configuration methods together.
//
// Example:
//
//	field := NewFieldBuilder("cluster name", "mycluster").
//	    Required().
//	    Help("lowercase letters and hyphens only").
//	    Validate(validateClusterName).
//	    Build()
type FieldBuilder struct {
	label       string
	placeholder string
	required    bool
	help        string
	password    bool
	validator   func(string) error
}

func NewFieldBuilder(label, placeholder string) *FieldBuilder {
	return &FieldBuilder{
		label:       label,
		placeholder: placeholder,
	}
}

func (b *FieldBuilder) Required() *FieldBuilder {
	b.required = true
	return b
}

func (b *FieldBuilder) Help(text string) *FieldBuilder {
	b.help = text
	return b
}

func (b *FieldBuilder) Password() *FieldBuilder {
	b.password = true
	return b
}

func (b *FieldBuilder) Validate(fn func(string) error) *FieldBuilder {
	b.validator = fn
	return b
}

func (b *FieldBuilder) Build() *components.InputField {
	var field *components.InputField
	if b.password {
		field = components.NewPasswordField(b.label, b.placeholder)
	} else {
		field = components.NewInputField(b.label, b.placeholder)
	}

	field.Required = b.required
	field.Help = b.help
	field.Validator = b.validator

	return field
}

func TextField(label, placeholder string) *FieldBuilder {
	return NewFieldBuilder(label, placeholder)
}

func PasswordField(label, placeholder string) *FieldBuilder {
	return NewFieldBuilder(label, placeholder).Password()
}

// SectionBuilder provides a fluent interface for building form sections.
// It collects fields and creates an InputGroup with a title.
//
// Example:
//
//	section := NewSectionBuilder("cluster identity").
//	    AddField(nameField).
//	    AddField(domainField).
//	    Build()
type SectionBuilder struct {
	title  string
	fields []*components.InputField
}

func NewSectionBuilder(title string) *SectionBuilder {
	return &SectionBuilder{
		title:  title,
		fields: make([]*components.InputField, 0),
	}
}

func (b *SectionBuilder) AddField(field *components.InputField) *SectionBuilder {
	b.fields = append(b.fields, field)
	return b
}

func (b *SectionBuilder) Field(fb *FieldBuilder) *SectionBuilder {
	return b.AddField(fb.Build())
}

func (b *SectionBuilder) Build() *components.InputGroup {
	return components.NewInputGroup(b.title, b.fields...)
}
