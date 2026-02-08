package config

import (
	"fmt"
	"strings"
)

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

type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

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

type ValidationOptions struct {
	Scope       ValidationScope
	StopOnFirst bool
}

type Validator interface {
	Validate(cfg *Config, result *ValidationResult)
	Scope() ValidationScope
}

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

// Validate uses ScopeAll when no options are provided.
func (cfg *Config) Validate(opts ...ValidationOptions) *ValidationResult {
	if len(opts) == 0 {
		opts = []ValidationOptions{{Scope: ScopeAll}}
	}
	return ValidateWithOptions(cfg, opts[0])
}

func (cfg *Config) ValidateWithScope(scope ValidationScope) *ValidationResult {
	return ValidateWithOptions(cfg, ValidationOptions{Scope: scope})
}

func (cfg *Config) ValidateQuick() *ValidationResult {
	return ValidateWithOptions(cfg, ValidationOptions{Scope: ScopeQuick})
}

func (cfg *Config) ValidateForOKD() *ValidationResult {
	return ValidateWithOKD(cfg)
}

func ValidateWithOptions(cfg *Config, opts ValidationOptions) *ValidationResult {
	registry := NewValidatorRegistry()
	return registry.Validate(cfg, opts)
}
