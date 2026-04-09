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
	msgs := make([]string, 0, len(r.Errors))
	for _, e := range r.Errors {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}

// ValidationScope controls what gets validated using a bitmask.
type ValidationScope uint64

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

func (s ValidationScope) HasScope(flag ValidationScope) bool {
	return s&flag != 0
}

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
	{ScopeFeatures, validateAddons},
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

func (cfg *Config) ValidateWithScope(scope ValidationScope) *ValidationResult {
	return ValidateWithOptions(cfg, ValidationOptions{Scope: scope})
}

func (cfg *Config) ValidateQuick() *ValidationResult {
	return ValidateWithOptions(cfg, ValidationOptions{Scope: ScopeQuick})
}

func ValidateWithOptions(cfg *Config, opts ValidationOptions) *ValidationResult {
	return runValidators(cfg, opts)
}
