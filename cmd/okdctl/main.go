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
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
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
			fields := []logutil.LogField{logutil.LF("value", v), logutil.LF("err", err)}
			if strings.HasPrefix(v, "~") && expanded == v {
				fields = append(fields, logutil.LF("note", "tilde expansion failed (home dir unresolved)"))
			}
			cli.DeferWarn(func() { logutil.Warn("OKDCTL_BIN_DIR ignored", fields...) })
		}
	}
	binDir := config.PreflightBinDir()
	path := os.Getenv("PATH")
	if !slices.Contains(filepath.SplitList(path), binDir) {
		if err := os.Setenv("PATH", binDir+":"+path); err != nil {
			cli.DeferWarn(func() {
				logutil.Warn("failed to prepend bin dir to PATH", logutil.LF("bin_dir", binDir), logutil.LF("err", err))
			})
		}
	}
}
