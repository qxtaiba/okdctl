// Package steps provides wizard step implementations for the TUI.
package steps

import (
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// ValidateNodeCount validates node count (1-100).
var ValidateNodeCount = config.ValidateNodeCount

// ═══════════════════════════════════════════════════════════════════════════════
// DOMAIN-SPECIFIC VALIDATORS
// ═══════════════════════════════════════════════════════════════════════════════

var (
	clusterNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)
	domainRegex      = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)
)

// ValidateClusterName validates Kubernetes naming conventions:
// 2-63 chars, starts with letter, lowercase letters/numbers/hyphens only.
func ValidateClusterName(value string) error {
	if len(value) < 2 {
		return errors.New("must be at least 2 characters")
	}
	if len(value) > 63 {
		return errors.New("must be at most 63 characters")
	}
	if !clusterNameRegex.MatchString(value) {
		return errors.New("must start with letter, contain only lowercase letters, numbers, hyphens")
	}
	return nil
}

// ValidateDomain validates DNS domain name format (3-253 chars).
func ValidateDomain(value string) error {
	if len(value) < 3 {
		return errors.New("must be at least 3 characters")
	}
	if len(value) > 253 {
		return errors.New("must be at most 253 characters")
	}
	if !domainRegex.MatchString(value) {
		return errors.New("invalid domain format")
	}
	return nil
}

// ValidateProxmoxHost validates a host address (IP or hostname, optionally with port).
func ValidateProxmoxHost(value string) error {
	host := value
	if strings.Contains(value, ":") {
		h, _, err := net.SplitHostPort(value)
		if err != nil {
			return errors.New("invalid host:port format")
		}
		host = h
	}

	if net.ParseIP(host) == nil {
		if len(host) == 0 || len(host) > 253 {
			return errors.New("invalid hostname")
		}
	}
	return nil
}

// ValidateYesNo validates a boolean-like string (yes/no/y/n/true/false/1/0).
func ValidateYesNo(value string) error {
	v := strings.ToLower(strings.TrimSpace(value))
	if v != "yes" && v != "no" && v != "y" && v != "n" && v != "true" && v != "false" && v != "1" && v != "0" {
		return errors.New("must be yes or no")
	}
	return nil
}

// ValidateFilePath validates that a file path exists (supports ~ expansion).
func ValidateFilePath(value string) error {
	if value == "" {
		return errors.New("path is required")
	}
	expanded := system.ExpandPath(value)

	if _, err := os.Stat(expanded); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", expanded)
	} else if err != nil {
		return utils.WrapError("cannot access file", err)
	}
	return nil
}
