package steps

import (
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

var ValidateNodeCount = config.ValidateNodeCount

var (
	clusterNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)
	domainRegex      = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)
)

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
		if host == "" || len(host) > 253 {
			return errors.New("invalid hostname")
		}
	}
	return nil
}

func ValidateFilePath(value string) error {
	if value == "" {
		return errors.New("path is required")
	}
	expanded := system.ExpandPath(value)

	if _, err := os.Stat(expanded); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", expanded)
	} else if err != nil {
		return fmt.Errorf("cannot access file: %w", err)
	}
	return nil
}
