package config

import (
	"fmt"
	"strings"
)

// Minimum resource thresholds. The *Generic values are used when no
// distribution-specific floor applies; OKD-specific floors apply when the
// distribution is set to OKD.
const (
	MinCPUGeneric      = 1
	MinMemoryMBGeneric = 1024
	MinDiskGBGeneric   = 20
	DefaultMinMemoryMB = 2048

	MinCPUControlPlaneOKD      = 4
	MinMemoryMBControlPlaneOKD = 8192
	MinDiskGBControlPlaneOKD   = 50

	MinCPUWorkerOKD      = 2
	MinMemoryMBWorkerOKD = 8192
	MinDiskGBWorkerOKD   = 50
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

// ValidationScope is a bitmask selecting which validators run. ScopeAll
// enables every validator; ScopeQuick runs the required/enum/networking
// set used during interactive editing.
type ValidationScope uint64

// Validation scope flags. Combine with bitwise-OR; ScopeAll enables
// every validator, ScopeQuick runs the required/enum/networking set
// used during interactive editing.
const (
	ScopeRequired ValidationScope = 1 << iota
	ScopeNetworking
	ScopeResources
	ScopeDistribution
	ScopeProvider
	ScopeFeatures
	ScopeFiles
	ScopeAdvancedNetworking
	ScopeHTTPServer
	ScopeEnums

	ScopeAll   = 0xFFFFFFFFFFFFFFFF
	ScopeQuick = ScopeRequired | ScopeEnums | ScopeNetworking
)

// HasScope reports whether s has flag set.
func (s ValidationScope) HasScope(flag ValidationScope) bool {
	return s&flag != 0
}

// ValidationOptions controls which validators run.
type ValidationOptions struct {
	Scope ValidationScope
}

type validatorEntry struct {
	scope    ValidationScope
	validate func(*Config, *ValidationResult)
}

var validators = []validatorEntry{
	{ScopeRequired, validateRequired},
	{ScopeEnums, validateEnums},
	{ScopeNetworking, validateNetworking},
	{ScopeAdvancedNetworking, validateAdvancedNetworking},
	{ScopeResources, validateResources},
	{ScopeProvider, validateProvider},
	{ScopeHTTPServer, validateHTTPServer},
	{ScopeDistribution, validateDistribution},
	{ScopeFiles, validateFiles},
}

func runValidators(cfg *Config, opts ValidationOptions) *ValidationResult {
	result := &ValidationResult{}

	if cfg == nil {
		result.AddError("_", "config is nil")
		return result
	}

	for _, v := range validators {
		if opts.Scope.HasScope(v.scope) {
			v.validate(cfg, result)
		}
	}

	return result
}

// Validate uses ScopeAll when no options are provided.
func (cfg *Config) Validate(opts ...ValidationOptions) *ValidationResult {
	if len(opts) == 0 {
		opts = []ValidationOptions{{Scope: ScopeAll}}
	}
	return ValidateWithOptions(cfg, opts[0])
}

// ValidateWithOptions runs the validator set selected by opts.Scope and
// returns the accumulated ValidationResult.
func ValidateWithOptions(cfg *Config, opts ValidationOptions) *ValidationResult {
	return runValidators(cfg, opts)
}
