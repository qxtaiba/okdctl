package steps

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/system"
)

// ValidateFilePath requires a non-empty path to an existing regular file —
// a directory passes os.Stat but fails an hour later at deploy time.
func ValidateFilePath(value string) error {
	if value == "" {
		return errors.New("path is required — enter a file path")
	}
	expanded := system.ExpandPath(value)

	info, err := os.Stat(expanded)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("file does not exist: %s — check the path", expanded)
	case err != nil:
		return fmt.Errorf("cannot access file: %w — check permissions", err)
	case info.IsDir():
		return fmt.Errorf("%s is a directory — point at the file itself", expanded)
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
