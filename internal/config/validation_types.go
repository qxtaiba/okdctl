package config

import (
	"fmt"
	"strings"
)

// Minimum resource thresholds: *Generic has no distribution-specific floor;
// OKD-specific floors apply when distribution is OKD.
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

// DefaultVMIDBase is the VMID assigned to the first cluster VM; config
// seeding and the proxmox post-apply probe both read it so they can't drift apart.
const DefaultVMIDBase = 6000

// DefaultOSDiskGB is the OS-disk size applied when topology.control_plane.
// disk_gb is 0; terraform rendering and the bootstrap disk validator both derive from it.
const DefaultOSDiskGB = 50

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

// ValidationScope is a bitmask selecting which validators ValidateWithOptions
// runs; ScopeAll enables every validator.
type ValidationScope uint64

// Validation scope flags. Combine with bitwise-OR.
const (
	ScopeRequired ValidationScope = 1 << iota
	ScopeNetworking
	ScopeResources
	ScopeDistribution
	ScopeProvider
	ScopeFiles
	ScopeAdvancedNetworking
	ScopeHTTPServer
	ScopeEnums
	ScopeDeployment

	ScopeAll = 0xFFFFFFFFFFFFFFFF
)

// HasScope reports whether s has flag set.
func (s ValidationScope) HasScope(flag ValidationScope) bool {
	return s&flag != 0
}

// ValidationOptions controls which validators run.
type ValidationOptions struct {
	Scope ValidationScope
	// ProjectRoot anchors filesystem checks (terraform environments dir);
	// empty resolves against cwd.
	ProjectRoot string
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
	{ScopeAdvancedNetworking, validateStaticIPCollisions},
	{ScopeResources, validateResources},
	{ScopeProvider, validateProvider},
	{ScopeHTTPServer, validateHTTPServer},
	{ScopeDistribution, ValidateOKDConfig},
	{ScopeFiles, validateFiles},
	{ScopeDeployment, validateDeployment},
}

// Validate runs all validators against cfg; for scoped validation use ValidateWithOptions.
func (cfg *Config) Validate() *ValidationResult {
	return ValidateWithOptions(cfg, ValidationOptions{Scope: ScopeAll})
}

// ValidateWithOptions runs the validator set selected by opts.Scope.
func ValidateWithOptions(cfg *Config, opts ValidationOptions) *ValidationResult {
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

	if opts.Scope.HasScope(ScopeEnums) {
		validateTerraformEnvDir(cfg, opts.ProjectRoot, result)
	}

	return result
}
