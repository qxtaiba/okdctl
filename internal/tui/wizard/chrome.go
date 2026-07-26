package wizard

import (
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
)

// FlowChrome parameterizes the per-flow header chrome: the tagline under
// the logo and the green context badge on the footer divider. Badge may be
// nil (no badge).
type FlowChrome struct {
	Tagline string
	Badge   func(cfg *config.Config) string
}

// DefaultChrome returns the configure wizard's chrome.
func DefaultChrome() FlowChrome {
	return FlowChrome{Tagline: "okd over proxmox, the easy way", Badge: distributionBadge}
}

func distributionBadge(cfg *config.Config) string {
	var parts []string
	if cfg.Distribution.Type != "" {
		parts = append(parts, string(cfg.Distribution.Type))
		if cfg.Distribution.Version != "" {
			parts[0] += " " + cfg.Distribution.Version
		}
	}
	return strings.Join(parts, " → ")
}
