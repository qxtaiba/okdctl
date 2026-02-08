package config

import (
	"fmt"
	"strings"
)

// ═══════════════════════════════════════════════════════════════════════════════
// RESOURCE LIMITS
// ═══════════════════════════════════════════════════════════════════════════════

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

// ═══════════════════════════════════════════════════════════════════════════════
// VALIDATION TYPES
// ═══════════════════════════════════════════════════════════════════════════════

// ValidationError represents a single validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationResult holds all validation errors.
type ValidationResult struct {
	Errors []ValidationError
}

func (r *ValidationResult) IsValid() bool {
	return len(r.Errors) == 0
}

func (r *ValidationResult) AddError(field, message string) {
	r.Errors = append(r.Errors, ValidationError{Field: field, Message: message})
}

func (r *ValidationResult) Error() string {
	if r.IsValid() {
		return ""
	}
	var msgs []string
	for _, e := range r.Errors {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}

// ═══════════════════════════════════════════════════════════════════════════════
// VALIDATION SCOPE
// ═══════════════════════════════════════════════════════════════════════════════

// ValidationScope controls what gets validated using a bitmask.
type ValidationScope uint64

const (
	ScopeRequired           ValidationScope = 1 << iota
	ScopeNetworking
	ScopeResources
	ScopeDistribution
	ScopeProvider
	ScopeFeatures
	ScopeFiles
	ScopeAdvancedNetworking
	ScopeHTTPServer
	ScopeEnums
	ScopeDependencies

	ScopeAll   = 0xFFFFFFFFFFFFFFFF
	ScopeQuick = ScopeRequired | ScopeEnums | ScopeNetworking
)

func (s ValidationScope) HasScope(flag ValidationScope) bool {
	return s&flag != 0
}

// ValidationOptions configures validation behavior.
type ValidationOptions struct {
	Scope       ValidationScope
	StopOnFirst bool
}

// ═══════════════════════════════════════════════════════════════════════════════
// VALIDATOR INTERFACE AND REGISTRY
// ═══════════════════════════════════════════════════════════════════════════════

// Validator interface for pluggable validation.
type Validator interface {
	Validate(cfg *Config, result *ValidationResult)
	Scope() ValidationScope
}

// ValidatorRegistry manages registered validators.
type ValidatorRegistry struct {
	validators []Validator
}

func NewValidatorRegistry() *ValidatorRegistry {
	return &ValidatorRegistry{
		validators: []Validator{
			&requiredFieldsValidator{},
			&enumsValidator{},
			&networkingValidator{},
			&advancedNetworkingValidator{},
			&resourcesValidator{},
			&providerValidator{},
			&addonsValidator{},
			&httpServerValidator{},
			&distributionValidator{},
			&filesValidator{},
		},
	}
}

func (r *ValidatorRegistry) Validate(cfg *Config, opts ValidationOptions) *ValidationResult {
	result := &ValidationResult{}

	for _, v := range r.validators {
		if opts.Scope.HasScope(v.Scope()) {
			v.Validate(cfg, result)
			if opts.StopOnFirst && !result.IsValid() {
				return result
			}
		}
	}

	return result
}

// ═══════════════════════════════════════════════════════════════════════════════
// CONFIG VALIDATION METHODS
// ═══════════════════════════════════════════════════════════════════════════════

// Validate validates the configuration. Optional ValidationOptions control scope and behavior.
// Defaults to ScopeAll when no options are provided.
func (cfg *Config) Validate(opts ...ValidationOptions) *ValidationResult {
	if len(opts) == 0 {
		opts = []ValidationOptions{{Scope: ScopeAll}}
	}
	return ValidateWithOptions(cfg, opts[0])
}

// ValidateWithScope validates with a specific scope.
func (cfg *Config) ValidateWithScope(scope ValidationScope) *ValidationResult {
	return ValidateWithOptions(cfg, ValidationOptions{Scope: scope})
}

// ValidateQuick performs fast validation without file I/O checks.
func (cfg *Config) ValidateQuick() *ValidationResult {
	return ValidateWithOptions(cfg, ValidationOptions{Scope: ScopeQuick})
}

// ValidateForOKD validates the configuration including OKD-specific resource requirements.
func (cfg *Config) ValidateForOKD() *ValidationResult {
	return ValidateWithOKD(cfg)
}

// ValidateWithOptions validates using the registry with specified options.
func ValidateWithOptions(cfg *Config, opts ValidationOptions) *ValidationResult {
	registry := NewValidatorRegistry()
	return registry.Validate(cfg, opts)
}
