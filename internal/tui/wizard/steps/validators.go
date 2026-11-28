package steps

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// ValidateFilePath requires a path to an existing regular file.
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

func validateIP(value string) error {
	if _, err := netip.ParseAddr(value); err != nil {
		return fmt.Errorf("invalid ip address %q — use e.g. 192.168.1.1", value)
	}
	return nil
}

func validateCIDR(value string) error {
	if _, err := netip.ParsePrefix(value); err != nil {
		return fmt.Errorf("invalid cidr format %q — use e.g. 192.168.1.0/24", value)
	}
	return nil
}

func validateIPList(value string) error {
	for _, entry := range splitList(value) {
		if err := validateIP(entry); err != nil {
			return err
		}
	}
	return nil
}

func validateDuration(value string) error {
	if _, err := time.ParseDuration(strings.TrimSpace(value)); err != nil {
		return fmt.Errorf("invalid duration %q — use go syntax such as 15m", value)
	}
	return nil
}

func minInt(lo int, unit string) func(string) error {
	return func(value string) error {
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("%q is not a whole number", value)
		}
		if n < lo {
			return fmt.Errorf("%d must be at least %d %s", n, lo, unit)
		}
		return nil
	}
}

func intRange(lo, hi int) func(string) error {
	return func(value string) error {
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("%q is not a whole number", value)
		}
		if n < lo || n > hi {
			return fmt.Errorf("%d must be between %d and %d", n, lo, hi)
		}
		return nil
	}
}

func splitList(value string) []string {
	var out []string
	for _, entry := range strings.Split(value, ",") {
		if entry = strings.TrimSpace(entry); entry != "" {
			out = append(out, entry)
		}
	}
	return out
}

func setList(set func(cfg *config.Config, v []string)) wizard.ConfigSetter {
	return wizard.SetString(func(cfg *config.Config, v string) { set(cfg, splitList(v)) })
}

func getList(get func(cfg *config.Config) []string) wizard.ConfigGetter {
	return func(cfg *config.Config) string { return strings.Join(get(cfg), ", ") }
}

func getBool(get func(cfg *config.Config) bool) wizard.ConfigGetter {
	return func(cfg *config.Config) string {
		if get(cfg) {
			return valYes
		}
		return valNo
	}
}

func setDuration(set func(cfg *config.Config, d time.Duration)) wizard.ConfigSetter {
	return func(cfg *config.Config, value string) error {
		d, err := time.ParseDuration(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("parse duration %q: %w", value, err)
		}
		set(cfg, d)
		return nil
	}
}

func getDuration(get func(cfg *config.Config) time.Duration) wizard.ConfigGetter {
	return func(cfg *config.Config) string { return durationText(get(cfg)) }
}

// durationText renders 15m0s as 15m, matching the yaml.
func durationText(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

func fieldLabel(key string) string {
	if i := strings.LastIndexByte(key, '.'); i >= 0 {
		key = key[i+1:]
	}
	return strings.ReplaceAll(key, "_", " ")
}
