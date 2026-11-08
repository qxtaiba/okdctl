package config

import (
	"fmt"
	"strings"
)

// ValidationError describes a single config validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationResult aggregates validation errors for a Config.
type ValidationResult struct {
	Errors []ValidationError
}

// IsValid reports whether the result has no errors.
func (r *ValidationResult) IsValid() bool {
	return len(r.Errors) == 0
}

// AddError appends a ValidationError built from field/message.
func (r *ValidationResult) AddError(field, message string) {
	r.Errors = append(r.Errors, ValidationError{Field: field, Message: message})
}

func (r *ValidationResult) Error() string {
	if r.IsValid() {
		return ""
	}
	msgs := make([]string, 0, len(r.Errors))
	for _, e := range r.Errors {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}

// Validate runs every validator against cfg and returns all findings.
func (cfg *Config) Validate() *ValidationResult {
	result := &ValidationResult{}
	if cfg == nil {
		result.AddError("_", "config is nil")
		return result
	}
	validateRequired(cfg, result)
	return result
}
