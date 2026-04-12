package steps

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/utils/system"
)

var (
	ValidateNodeCount   = config.ValidateNodeCount
	ValidateClusterName = config.ValidateClusterName
	ValidateDomain      = config.ValidateDomain
	ValidateProxmoxHost = config.ValidateProxmoxHost
)

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

func validateDNSServers(value string) error {
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if err := config.ValidateIP(entry); err != nil {
			return fmt.Errorf("invalid dns server %q: %w", entry, err)
		}
	}
	return nil
}
