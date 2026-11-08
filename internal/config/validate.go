package config

import (
	"fmt"
	"strings"
)

// ValidationError is one finding against a dotted Config path.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// ValidationResult collects every finding of Validate.
type ValidationResult struct {
	Errors []ValidationError
}

// IsValid reports whether no finding was recorded.
func (r *ValidationResult) IsValid() bool {
	return len(r.Errors) == 0
}

// AddError records a finding against field.
func (r *ValidationResult) AddError(field, message string) {
	r.Errors = append(r.Errors, ValidationError{Field: field, Message: message})
}

func (r *ValidationResult) addf(field, format string, args ...any) {
	r.AddError(field, fmt.Sprintf(format, args...))
}

func (r *ValidationResult) Error() string {
	msgs := make([]string, 0, len(r.Errors))
	for _, e := range r.Errors {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}

// Validate checks every section of c and returns all findings, never only
// the first.
func (c *Config) Validate() *ValidationResult {
	r := &ValidationResult{}
	if c == nil {
		r.AddError("_", "config is nil")
		return r
	}
	c.validateCluster(r)
	c.validateProvider(r)
	c.validateNetwork(r)
	c.validateNodes(r)
	return r
}
