package wizard

import (
	"github.com/qxtaiba/okdctl/internal/config"
)

// FlowChrome parameterizes per-flow header chrome — tagline plus an
// optional context badge on the footer divider; Badge may be nil.
type FlowChrome struct {
	Tagline string
	Badge   func(cfg *config.Config) string
}

// DefaultChrome returns the configure wizard's chrome.
func DefaultChrome() FlowChrome {
	return FlowChrome{Tagline: "okd over proxmox, the easy way", Badge: distributionBadge}
}

func distributionBadge(cfg *config.Config) string {
	if cfg.Distribution.Type == "" {
		return ""
	}
	badge := string(cfg.Distribution.Type)
	if cfg.Distribution.Version != "" {
		badge += " " + cfg.Distribution.Version
	}
	return badge
}
