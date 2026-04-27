// Package main is the entry point for the okdctl binary, which deploys
// OKD clusters on Proxmox. Root-rejection policy lives in internal/cli
// (ensureRoot) so it can distinguish commands that require root from those
// that do not.
package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	_ "github.com/qxtaiba/okdctl/internal/addon/catalog" // Register all built-in addons
	"github.com/qxtaiba/okdctl/internal/cli"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

func main() {
	preflight()
	cli.Execute()
}

// preflight runs startup checks that must happen before the cobra command
// tree runs. It intentionally lives in main() rather than init() so tui
// output is available when reporting errors. Signal handling is installed
// inside cli.Execute; a Ctrl-C during preflight exits via the default
// SIGINT disposition (no summary printed), which is acceptable given
// preflight's sub-second runtime.
func preflight() {
	// Warn before OKDCTL_BIN_DIR is silently dropped downstream.
	if v := os.Getenv("OKDCTL_BIN_DIR"); v != "" {
		expanded := system.ExpandPath(v)
		if err := config.ValidateBinDir(expanded); err != nil {
			detail := err.Error()
			if strings.HasPrefix(v, "~") && expanded == v {
				detail = "tilde expansion failed (home dir unresolved); " + detail
			}
			tui.Warn("OKDCTL_BIN_DIR ignored", tui.LF("value", v), tui.LF("err", detail))
		}
	}
	binDir := phase.PreflightBinDir()
	path := os.Getenv("PATH")
	if !slices.Contains(filepath.SplitList(path), binDir) {
		if err := os.Setenv("PATH", binDir+":"+path); err != nil {
			tui.Warn("failed to prepend bin dir to PATH", tui.LF("bin_dir", binDir), tui.LF("err", err))
		}
	}
}
